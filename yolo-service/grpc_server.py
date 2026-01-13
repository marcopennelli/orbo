#!/usr/bin/env python3
"""
gRPC server for YOLO detection service
Provides bidirectional streaming for low-latency real-time detection
"""

import grpc
from concurrent import futures
import time
import logging
import os
import threading
from typing import Iterator, Dict, Any, Optional, List

# Proto imports - generated from api/proto/detection/v1/detection.proto
# Run: python -m grpc_tools.protoc -I../../api/proto --python_out=. --grpc_python_out=. ../../api/proto/detection/v1/detection.proto
import detection_pb2
import detection_pb2_grpc

logger = logging.getLogger(__name__)


class DetectionServicer(detection_pb2_grpc.DetectionServiceServicer):
    """gRPC servicer for YOLO object detection"""

    def __init__(self, detection_service):
        """
        Args:
            detection_service: YOLODetectionService instance from main.py
        """
        self.service = detection_service
        self.active_streams = 0
        self.streams_lock = threading.Lock()

        # Configuration
        self.conf_threshold = float(os.getenv('CONF_THRESHOLD', '0.5'))
        self.iou_threshold = float(os.getenv('IOU_THRESHOLD', '0.45'))
        self.tracking_enabled = os.getenv('TRACKER_TYPE', '') != ''
        self.tracker_type = os.getenv('TRACKER_TYPE', 'bytetrack')

        # Classes filter - list of class names to detect (None = all classes)
        # Can be set via Configure RPC or CLASSES_FILTER env var
        self.classes_filter: Optional[List[str]] = None
        classes_env = os.getenv('CLASSES_FILTER', '')
        if classes_env:
            self.classes_filter = [c.strip().lower() for c in classes_env.split(',') if c.strip()]
            logger.info(f"[gRPC] Initial classes filter from env: {self.classes_filter}")

    def DetectStream(
        self,
        request_iterator: Iterator[detection_pb2.FrameRequest],
        context: grpc.ServicerContext
    ) -> Iterator[detection_pb2.DetectionResponse]:
        """
        Bidirectional streaming RPC for real-time detection.
        Receives frames, returns detection results with minimal latency.
        """
        with self.streams_lock:
            self.active_streams += 1
            stream_id = self.active_streams

        logger.info(f"[gRPC] Stream {stream_id} started")

        try:
            for request in request_iterator:
                start_time = time.time()

                try:
                    # Determine if tracking is requested and available
                    use_tracking = request.enable_tracking and self.tracking_enabled
                    conf_threshold = request.conf_threshold if request.conf_threshold > 0 else self.conf_threshold

                    # Run detection based on tracking and annotation mode
                    # Use configured classes filter (from Configure RPC or env var)
                    if use_tracking:
                        if request.return_annotated:
                            annotated_jpeg, result_info = self.service.detect_and_annotate_with_tracking(
                                request.jpeg_data,
                                camera_id=request.camera_id,
                                conf_threshold=conf_threshold,
                                classes_filter=self.classes_filter,
                                show_labels=True,
                                show_confidence=True
                            )
                        else:
                            result_info = self.service.detect_with_tracking(
                                request.jpeg_data,
                                camera_id=request.camera_id,
                                conf_threshold=conf_threshold,
                                classes_filter=self.classes_filter
                            )
                            annotated_jpeg = b''
                    else:
                        if request.return_annotated:
                            annotated_jpeg, result_info = self.service.detect_and_annotate(
                                request.jpeg_data,
                                conf_threshold=conf_threshold,
                                classes_filter=self.classes_filter,
                                show_labels=True,
                                show_confidence=True
                            )
                        else:
                            result_info = self.service.detect_objects(
                                request.jpeg_data,
                                conf_threshold=conf_threshold,
                                classes_filter=self.classes_filter
                            )
                            annotated_jpeg = b''

                    # Build response
                    response = detection_pb2.DetectionResponse(
                        camera_id=request.camera_id,
                        frame_seq=request.frame_seq,
                        capture_timestamp_ns=request.timestamp_ns,
                        inference_timestamp_ns=int(time.time() * 1e9),
                        annotated_jpeg=annotated_jpeg if request.return_annotated else b'',
                        inference_ms=result_info.get('inference_time_ms', 0),
                        device=str(self.service.device)
                    )

                    # Add detections
                    for det in result_info.get('detections', []):
                        bbox = det.get('bbox', [0, 0, 0, 0])
                        detection = detection_pb2.Detection(
                            class_name=det.get('class', ''),
                            class_id=det.get('class_id', 0),
                            confidence=det.get('confidence', 0),
                            bbox=detection_pb2.BBox(
                                x1=bbox[0] if len(bbox) > 0 else 0,
                                y1=bbox[1] if len(bbox) > 1 else 0,
                                x2=bbox[2] if len(bbox) > 2 else 0,
                                y2=bbox[3] if len(bbox) > 3 else 0
                            ),
                            track_id=det.get('track_id', 0),
                            velocity_x=det.get('velocity_x', 0.0),
                            velocity_y=det.get('velocity_y', 0.0),
                            threat_level=self._get_threat_level(det.get('class', ''))
                        )
                        response.detections.append(detection)

                    # Add track updates if tracking is enabled
                    for track in result_info.get('tracks', []):
                        track_update = detection_pb2.TrackUpdate(
                            track_id=track.get('track_id', 0),
                            state=track.get('state', 'unknown'),
                            age=track.get('age', 0),
                            time_since_update=track.get('time_since_update', 0)
                        )
                        response.tracks.append(track_update)

                    yield response

                except Exception as e:
                    logger.error(f"[gRPC] Stream {stream_id} detection error: {e}")
                    # Continue processing next frame instead of breaking stream
                    continue

        except Exception as e:
            logger.error(f"[gRPC] Stream {stream_id} error: {e}")
        finally:
            with self.streams_lock:
                self.active_streams -= 1
            logger.info(f"[gRPC] Stream {stream_id} ended")

    def HealthCheck(
        self,
        request: detection_pb2.HealthRequest,
        context: grpc.ServicerContext
    ) -> detection_pb2.HealthResponse:
        """Check service health status"""
        return detection_pb2.HealthResponse(
            status="healthy" if self.service.model_loaded else "unhealthy",
            device=str(self.service.device),
            model_loaded=self.service.model_loaded,
            tracker_type=self.tracker_type if self.tracking_enabled else "",
            model_name=os.getenv('YOLO_MODEL', 'yolo11n.pt'),
            active_streams=self.active_streams
        )

    def Configure(
        self,
        request: detection_pb2.ConfigureRequest,
        context: grpc.ServicerContext
    ) -> detection_pb2.ConfigureResponse:
        """Update detection configuration at runtime"""
        try:
            if request.HasField('conf_threshold'):
                self.conf_threshold = request.conf_threshold
            if request.HasField('iou_threshold'):
                self.iou_threshold = request.iou_threshold
            if request.HasField('enable_tracking'):
                self.tracking_enabled = request.enable_tracking
            if request.HasField('tracker_type'):
                self.tracker_type = request.tracker_type

            # Handle classes filter from repeated string field
            # Empty list means "all classes", non-empty means filter to those classes
            if len(request.classes) > 0:
                self.classes_filter = [c.strip().lower() for c in request.classes if c.strip()]
                logger.info(f"[gRPC] Classes filter updated: {self.classes_filter}")
            elif request.classes is not None and len(request.classes) == 0:
                # Explicit empty array means clear the filter (detect all)
                self.classes_filter = None
                logger.info("[gRPC] Classes filter cleared (detecting all classes)")

            # Handle bounding box color configuration
            if request.HasField('box_color'):
                self.service.set_box_color(request.box_color)
            if request.HasField('box_thickness'):
                self.service.set_box_thickness(request.box_thickness)

            logger.info(f"[gRPC] Configuration updated: conf={self.conf_threshold}, "
                       f"tracking={self.tracking_enabled}, tracker={self.tracker_type}, "
                       f"classes={self.classes_filter}")

            return detection_pb2.ConfigureResponse(
                success=True,
                message="Configuration updated",
                conf_threshold=self.conf_threshold,
                iou_threshold=self.iou_threshold,
                tracking_enabled=self.tracking_enabled,
                tracker_type=self.tracker_type
            )
        except Exception as e:
            return detection_pb2.ConfigureResponse(
                success=False,
                message=str(e)
            )

    def _get_threat_level(self, class_name: str) -> str:
        """Determine threat level based on detected class"""
        if class_name == "person":
            return "high"
        elif class_name in ["car", "truck", "bus"]:
            return "medium"
        else:
            return "low"

    def _proto_task_to_string(self, task: int) -> str:
        """Convert proto YoloTask enum to string"""
        task_map = {
            0: "detect",  # YOLO_TASK_UNSPECIFIED
            1: "detect",  # YOLO_TASK_DETECT
            2: "pose",    # YOLO_TASK_POSE
            3: "segment", # YOLO_TASK_SEGMENT
            4: "obb",     # YOLO_TASK_OBB
            5: "classify" # YOLO_TASK_CLASSIFY
        }
        return task_map.get(task, "detect")

    def AnalyzeStream(
        self,
        request_iterator: Iterator[detection_pb2.AnalyzeRequest],
        context: grpc.ServicerContext
    ) -> Iterator[detection_pb2.AnalyzeResponse]:
        """
        Bidirectional streaming RPC for multi-task YOLO11 analysis.
        Supports: detect, pose, segment, obb, classify
        """
        with self.streams_lock:
            self.active_streams += 1
            stream_id = self.active_streams

        logger.info(f"[gRPC] AnalyzeStream {stream_id} started")

        try:
            for request in request_iterator:
                start_time = time.time()

                try:
                    # Convert proto tasks to string list
                    tasks = [self._proto_task_to_string(t) for t in request.tasks]
                    if not tasks:
                        tasks = ["detect"]

                    logger.info(f"[gRPC] AnalyzeStream {stream_id} request: tasks={tasks}, camera={request.camera_id}")

                    conf_threshold = request.conf_threshold if request.conf_threshold > 0 else self.conf_threshold

                    # Convert classes filter
                    classes_filter = list(request.classes_filter) if request.classes_filter else self.classes_filter

                    # Run multi-task analysis
                    result = self.service.analyze(
                        image_data=request.jpeg_data,
                        tasks=tasks,
                        conf_threshold=conf_threshold,
                        classes_filter=classes_filter,
                        return_annotated=request.return_annotated
                    )

                    # Build response
                    response = detection_pb2.AnalyzeResponse(
                        camera_id=request.camera_id,
                        frame_seq=request.frame_seq,
                        capture_timestamp_ns=request.timestamp_ns,
                        inference_timestamp_ns=int(time.time() * 1e9),
                        total_inference_ms=result.get('inference_time_ms', 0),
                        device=result.get('device', str(self.service.device))
                    )

                    # Add annotated image if requested
                    if request.return_annotated and 'annotated_image' in result:
                        import base64
                        response.annotated_jpeg = base64.b64decode(result['annotated_image'])

                    # Add detection results
                    task_results = result.get('tasks', {})

                    if 'detect' in task_results:
                        detect_result = task_results['detect']
                        detect_pb = detection_pb2.TaskResults(
                            count=detect_result.get('count', 0),
                            inference_ms=result.get('inference_time_ms', 0) / max(len(tasks), 1)
                        )
                        for det in detect_result.get('detections', []):
                            bbox = det.get('bbox', [0, 0, 0, 0])
                            detection = detection_pb2.Detection(
                                class_name=det.get('class', ''),
                                class_id=det.get('class_id', 0),
                                confidence=det.get('confidence', 0),
                                bbox=detection_pb2.BBox(
                                    x1=bbox[0] if len(bbox) > 0 else 0,
                                    y1=bbox[1] if len(bbox) > 1 else 0,
                                    x2=bbox[2] if len(bbox) > 2 else 0,
                                    y2=bbox[3] if len(bbox) > 3 else 0
                                ),
                                threat_level=self._get_threat_level(det.get('class', ''))
                            )
                            detect_pb.detections.append(detection)
                        response.detect.CopyFrom(detect_pb)

                    if 'pose' in task_results:
                        pose_result = task_results['pose']
                        pose_pb = detection_pb2.PoseResults(
                            count=pose_result.get('count', 0),
                            inference_ms=result.get('inference_time_ms', 0) / max(len(tasks), 1)
                        )
                        for pose in pose_result.get('poses', []):
                            bbox = pose.get('bbox') or [0, 0, 0, 0]
                            pose_det = detection_pb2.PoseDetection(
                                bbox=detection_pb2.BBox(
                                    x1=bbox[0] if len(bbox) > 0 else 0,
                                    y1=bbox[1] if len(bbox) > 1 else 0,
                                    x2=bbox[2] if len(bbox) > 2 else 0,
                                    y2=bbox[3] if len(bbox) > 3 else 0
                                ),
                                confidence=pose.get('confidence', 0),
                                pose_class=pose.get('pose_class', 'unknown')
                            )
                            # Add keypoints
                            keypoints = pose.get('keypoints', [])
                            keypoint_conf = pose.get('keypoint_confidence', [])
                            keypoint_names = pose.get('keypoint_names', [])
                            for i, kp in enumerate(keypoints):
                                if len(kp) >= 2:
                                    kp_name = keypoint_names[i] if i < len(keypoint_names) else f'kp_{i}'
                                    kp_conf = keypoint_conf[i] if i < len(keypoint_conf) else 0.0
                                    keypoint = detection_pb2.Keypoint(
                                        x=kp[0],
                                        y=kp[1],
                                        confidence=kp_conf,
                                        name=kp_name
                                    )
                                    pose_det.keypoints.append(keypoint)
                            pose_pb.poses.append(pose_det)
                        response.pose.CopyFrom(pose_pb)

                    if 'segment' in task_results:
                        seg_result = task_results['segment']
                        seg_pb = detection_pb2.SegmentResults(
                            count=seg_result.get('count', 0),
                            inference_ms=result.get('inference_time_ms', 0) / max(len(tasks), 1)
                        )
                        for seg in seg_result.get('segments', []):
                            bbox = seg.get('bbox', [0, 0, 0, 0])
                            seg_det = detection_pb2.SegmentDetection(
                                class_name=seg.get('class', ''),
                                class_id=seg.get('class_id', 0),
                                confidence=seg.get('confidence', 0),
                                bbox=detection_pb2.BBox(
                                    x1=bbox[0] if len(bbox) > 0 else 0,
                                    y1=bbox[1] if len(bbox) > 1 else 0,
                                    x2=bbox[2] if len(bbox) > 2 else 0,
                                    y2=bbox[3] if len(bbox) > 3 else 0
                                )
                            )
                            # Add mask polygon if available (polygon is list of [x, y] pairs)
                            polygon = seg.get('polygon', [])
                            for point in polygon:
                                if isinstance(point, (list, tuple)) and len(point) >= 2:
                                    seg_det.mask_polygon.append(float(point[0]))
                                    seg_det.mask_polygon.append(float(point[1]))
                                elif isinstance(point, (int, float)):
                                    seg_det.mask_polygon.append(float(point))
                            seg_pb.segments.append(seg_det)
                        response.segment.CopyFrom(seg_pb)

                    if 'obb' in task_results:
                        obb_result = task_results['obb']
                        obb_pb = detection_pb2.OBBResults(
                            count=obb_result.get('count', 0),
                            inference_ms=result.get('inference_time_ms', 0) / max(len(tasks), 1)
                        )
                        for obb in obb_result.get('obbs', []):
                            obb_det = detection_pb2.OBBDetection(
                                class_name=obb.get('class', ''),
                                class_id=obb.get('class_id', 0),
                                confidence=obb.get('confidence', 0),
                                cx=obb.get('center', [0, 0])[0],
                                cy=obb.get('center', [0, 0])[1],
                                width=obb.get('width', 0),
                                height=obb.get('height', 0),
                                angle=obb.get('angle', 0)
                            )
                            for corner in obb.get('corners', []):
                                obb_det.corners.append(float(corner))
                            obb_pb.obbs.append(obb_det)
                        response.obb.CopyFrom(obb_pb)

                    if 'classify' in task_results:
                        cls_result = task_results['classify']
                        cls_pb = detection_pb2.ClassifyResults(
                            inference_ms=result.get('inference_time_ms', 0) / max(len(tasks), 1)
                        )
                        for cls in cls_result.get('classifications', []):
                            classification = detection_pb2.Classification(
                                class_name=cls.get('class', ''),
                                class_id=cls.get('class_id', 0),
                                confidence=cls.get('confidence', 0)
                            )
                            cls_pb.classifications.append(classification)
                        response.classify.CopyFrom(cls_pb)

                    # Add alerts from pose analysis
                    for alert in result.get('alerts', []):
                        response.alerts.append(alert.get('type', ''))

                    yield response

                except Exception as e:
                    logger.error(f"[gRPC] AnalyzeStream {stream_id} detection error: {e}")
                    import traceback
                    traceback.print_exc()
                    continue

        except Exception as e:
            logger.error(f"[gRPC] AnalyzeStream {stream_id} error: {e}")
        finally:
            with self.streams_lock:
                self.active_streams -= 1
            logger.info(f"[gRPC] AnalyzeStream {stream_id} ended")


def serve(detection_service, port: int = 50051, max_workers: int = 4):
    """
    Start the gRPC server.

    Args:
        detection_service: YOLODetectionService instance
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

    servicer = DetectionServicer(detection_service)
    detection_pb2_grpc.add_DetectionServiceServicer_to_server(servicer, server)

    server.add_insecure_port(f'[::]:{port}')
    server.start()

    logger.info(f"[gRPC] Detection server started on port {port}")

    return server


if __name__ == "__main__":
    # For standalone testing - normally integrated with main.py
    logging.basicConfig(level=logging.INFO)

    # Import the detection service from main
    from main import YOLODetectionService

    detection_service = YOLODetectionService()

    port = int(os.getenv('GRPC_PORT', '50051'))
    server = serve(detection_service, port=port)

    logger.info(f"gRPC server running on port {port}")

    try:
        server.wait_for_termination()
    except KeyboardInterrupt:
        server.stop(0)
