#!/usr/bin/env python3
"""
gRPC server for Face Recognition service
Provides bidirectional streaming for low-latency real-time face recognition
"""

import grpc
from concurrent import futures
import time
import logging
import os
import threading
from typing import Iterator, Dict, Any, Optional, List
import numpy as np

# Proto imports - generated from api/proto/recognition/v1/recognition.proto
import recognition_pb2
import recognition_pb2_grpc

logger = logging.getLogger(__name__)


class FaceRecognitionServicer(recognition_pb2_grpc.FaceRecognitionServiceServicer):
    """gRPC servicer for InsightFace face recognition"""

    def __init__(self, recognition_service):
        """
        Args:
            recognition_service: FaceRecognitionService instance from main.py
        """
        self.service = recognition_service
        self.active_streams = 0
        self.streams_lock = threading.Lock()

        # Configuration
        self.similarity_threshold = float(os.getenv('SIMILARITY_THRESHOLD', '0.5'))

    def RecognizeStream(
        self,
        request_iterator: Iterator[recognition_pb2.FaceRequest],
        context: grpc.ServicerContext
    ) -> Iterator[recognition_pb2.FaceResponse]:
        """
        Bidirectional streaming RPC for real-time face recognition.
        Receives frames, returns face recognition results with minimal latency.
        """
        with self.streams_lock:
            self.active_streams += 1
            stream_id = self.active_streams

        logger.info(f"[gRPC] Face recognition stream {stream_id} started")

        try:
            for request in request_iterator:
                start_time = time.time()

                try:
                    # Determine similarity threshold
                    threshold = request.similarity_threshold if request.similarity_threshold > 0 else self.similarity_threshold

                    # Check if we have person regions from YOLO
                    if request.person_regions:
                        # Optimize: only search for faces within person bounding boxes
                        result_info = self._recognize_in_regions(
                            request.jpeg_data,
                            request.person_regions,
                            request.person_track_ids,
                            threshold
                        )
                        annotated_jpeg = b''
                    elif request.return_annotated:
                        annotated_jpeg, result_info = self.service.detect_and_annotate(
                            request.jpeg_data,
                            recognize=True
                        )
                    else:
                        result_info = self.service.recognize_faces(request.jpeg_data)
                        annotated_jpeg = b''

                    inference_time = time.time() - start_time

                    # Build response
                    response = recognition_pb2.FaceResponse(
                        camera_id=request.camera_id,
                        frame_seq=request.frame_seq,
                        capture_timestamp_ns=request.timestamp_ns,
                        inference_timestamp_ns=int(time.time() * 1e9),
                        annotated_jpeg=annotated_jpeg if request.return_annotated else b'',
                        inference_ms=result_info.get('inference_time_ms', inference_time * 1000),
                        device=str(self.service.device),
                        total_count=result_info.get('count', 0),
                        known_count=result_info.get('known_count', 0),
                        unknown_count=result_info.get('unknown_count', 0)
                    )

                    # Add face recognitions
                    for rec in result_info.get('recognitions', []):
                        bbox = rec.get('bbox', [0, 0, 0, 0])
                        face = recognition_pb2.FaceRecognition(
                            bbox=recognition_pb2.BBox(
                                x1=float(bbox[0]) if len(bbox) > 0 else 0,
                                y1=float(bbox[1]) if len(bbox) > 1 else 0,
                                x2=float(bbox[2]) if len(bbox) > 2 else 0,
                                y2=float(bbox[3]) if len(bbox) > 3 else 0
                            ),
                            confidence=rec.get('confidence', 0),
                            identity=rec.get('identity') or '',
                            similarity=rec.get('similarity', 0),
                            is_known=rec.get('is_known', False),
                            age=rec.get('age', 0),
                            gender=rec.get('gender', ''),
                            associated_track_id=rec.get('associated_track_id', 0)
                        )
                        response.faces.append(face)

                    yield response

                except Exception as e:
                    logger.error(f"[gRPC] Stream {stream_id} recognition error: {e}")
                    # Continue processing next frame instead of breaking stream
                    continue

        except Exception as e:
            logger.error(f"[gRPC] Stream {stream_id} error: {e}")
        finally:
            with self.streams_lock:
                self.active_streams -= 1
            logger.info(f"[gRPC] Face recognition stream {stream_id} ended")

    def _recognize_in_regions(
        self,
        jpeg_data: bytes,
        person_regions: List[recognition_pb2.BBox],
        track_ids: List[int],
        threshold: float
    ) -> Dict[str, Any]:
        """
        Recognize faces only within specified person bounding boxes.
        This is faster than full-frame recognition when YOLO has already detected persons.
        """
        start_time = time.time()

        try:
            image = self.service.preprocess_image(jpeg_data)
            all_recognitions = []

            for i, region in enumerate(person_regions):
                # Extract person crop
                x1 = max(0, int(region.x1))
                y1 = max(0, int(region.y1))
                x2 = min(image.shape[1], int(region.x2))
                y2 = min(image.shape[0], int(region.y2))

                if x2 <= x1 or y2 <= y1:
                    continue

                person_crop = image[y1:y2, x1:x2]

                # Detect faces in person crop
                faces = self.service.face_app.get(person_crop)

                track_id = track_ids[i] if i < len(track_ids) else 0

                for face in faces:
                    # Adjust bbox to full frame coordinates
                    bbox = face.bbox.tolist()
                    adjusted_bbox = [
                        bbox[0] + x1,
                        bbox[1] + y1,
                        bbox[2] + x1,
                        bbox[3] + y1
                    ]

                    recognition = {
                        "bbox": adjusted_bbox,
                        "confidence": float(face.det_score),
                        "identity": None,
                        "similarity": 0.0,
                        "is_known": False,
                        "associated_track_id": track_id
                    }

                    # Add age/gender if available
                    if hasattr(face, 'age') and face.age is not None:
                        recognition["age"] = int(face.age)
                    if hasattr(face, 'gender') and face.gender is not None:
                        recognition["gender"] = "male" if face.gender == 1 else "female"

                    # Match against known faces
                    if face.embedding is not None and len(self.service.known_faces) > 0:
                        best_match = None
                        best_similarity = 0.0

                        for name, data in self.service.known_faces.items():
                            similarity = self.service._compute_similarity(face.embedding, data['embedding'])
                            if similarity > best_similarity:
                                best_similarity = similarity
                                best_match = name

                        if best_similarity >= threshold:
                            recognition["identity"] = best_match
                            recognition["similarity"] = round(best_similarity, 3)
                            recognition["is_known"] = True

                    all_recognitions.append(recognition)

            inference_time = time.time() - start_time

            known_count = sum(1 for r in all_recognitions if r["is_known"])

            return {
                "recognitions": all_recognitions,
                "count": len(all_recognitions),
                "known_count": known_count,
                "unknown_count": len(all_recognitions) - known_count,
                "inference_time_ms": round(inference_time * 1000, 2),
                "device": self.service.device
            }

        except Exception as e:
            logger.error(f"Region-based recognition failed: {e}")
            raise

    def RegisterFace(
        self,
        request: recognition_pb2.RegisterFaceRequest,
        context: grpc.ServicerContext
    ) -> recognition_pb2.RegisterFaceResponse:
        """Register a new face identity"""
        try:
            result = self.service.register_face(request.name, request.image_data)

            return recognition_pb2.RegisterFaceResponse(
                success=result.get('success', False),
                message=result.get('message', ''),
                name=result.get('name', request.name),
                face_count=len(self.service.known_faces)
            )
        except Exception as e:
            return recognition_pb2.RegisterFaceResponse(
                success=False,
                message=str(e),
                name=request.name,
                face_count=len(self.service.known_faces)
            )

    def DeleteFace(
        self,
        request: recognition_pb2.DeleteFaceRequest,
        context: grpc.ServicerContext
    ) -> recognition_pb2.DeleteFaceResponse:
        """Delete a face identity"""
        try:
            result = self.service.delete_face(request.name)

            return recognition_pb2.DeleteFaceResponse(
                success=result.get('success', False),
                message=result.get('message', ''),
                face_count=len(self.service.known_faces)
            )
        except Exception as e:
            return recognition_pb2.DeleteFaceResponse(
                success=False,
                message=str(e),
                face_count=len(self.service.known_faces)
            )

    def ListFaces(
        self,
        request: recognition_pb2.ListFacesRequest,
        context: grpc.ServicerContext
    ) -> recognition_pb2.ListFacesResponse:
        """List all registered face identities"""
        result = self.service.list_faces()

        response = recognition_pb2.ListFacesResponse(
            count=result.get('count', 0)
        )

        for face in result.get('faces', []):
            face_identity = recognition_pb2.FaceIdentity(
                name=face.get('name', ''),
                has_image=face.get('has_image', False),
                age=face.get('age', 0),
                gender=face.get('gender', '')
            )
            # Parse created_at if present
            if face.get('created_at'):
                try:
                    from datetime import datetime
                    dt = datetime.fromisoformat(face['created_at'])
                    face_identity.created_at_ns = int(dt.timestamp() * 1e9)
                except:
                    pass
            response.faces.append(face_identity)

        return response

    def HealthCheck(
        self,
        request: recognition_pb2.HealthRequest,
        context: grpc.ServicerContext
    ) -> recognition_pb2.HealthResponse:
        """Check service health status"""
        return recognition_pb2.HealthResponse(
            status="healthy" if self.service.model_loaded else "unhealthy",
            device=str(self.service.device),
            model_loaded=self.service.model_loaded,
            known_faces_count=len(self.service.known_faces),
            model_name=os.getenv('INSIGHTFACE_MODEL', 'buffalo_l')
        )

    def Configure(
        self,
        request: recognition_pb2.ConfigureRequest,
        context: grpc.ServicerContext
    ) -> recognition_pb2.ConfigureResponse:
        """Update recognition configuration at runtime"""
        try:
            if request.HasField('similarity_threshold'):
                self.similarity_threshold = request.similarity_threshold
                self.service.similarity_threshold = request.similarity_threshold

            logger.info(f"[gRPC] Configuration updated: similarity_threshold={self.similarity_threshold}")

            return recognition_pb2.ConfigureResponse(
                success=True,
                message="Configuration updated",
                similarity_threshold=self.similarity_threshold,
                age_gender_enabled=True  # InsightFace always has age/gender
            )
        except Exception as e:
            return recognition_pb2.ConfigureResponse(
                success=False,
                message=str(e)
            )


def serve(recognition_service, port: int = 50052, max_workers: int = 4):
    """
    Start the gRPC server.

    Args:
        recognition_service: FaceRecognitionService instance
        port: Port to listen on
        max_workers: Maximum number of worker threads
    """
    server = grpc.server(
        futures.ThreadPoolExecutor(max_workers=max_workers),
        options=[
            ('grpc.max_send_message_length', 50 * 1024 * 1024),  # 50MB
            ('grpc.max_receive_message_length', 50 * 1024 * 1024),  # 50MB
            ('grpc.keepalive_time_ms', 10000),  # 10 seconds
            ('grpc.keepalive_timeout_ms', 5000),  # 5 seconds
            ('grpc.keepalive_permit_without_calls', True),
            ('grpc.http2.max_pings_without_data', 0),
        ]
    )

    servicer = FaceRecognitionServicer(recognition_service)
    recognition_pb2_grpc.add_FaceRecognitionServiceServicer_to_server(servicer, server)

    server.add_insecure_port(f'[::]:{port}')
    server.start()

    logger.info(f"[gRPC] Face Recognition server started on port {port}")

    return server


if __name__ == "__main__":
    # For standalone testing - normally integrated with main.py
    logging.basicConfig(level=logging.INFO)

    # Import the recognition service from main
    from main import FaceRecognitionService

    recognition_service = FaceRecognitionService()

    port = int(os.getenv('GRPC_PORT', '50052'))
    server = serve(recognition_service, port=port)

    logger.info(f"gRPC server running on port {port}")

    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(0)
