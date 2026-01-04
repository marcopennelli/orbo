// Face Recognition Service API Client
// The recognition service runs on port 8082

const RECOGNITION_BASE = '/recognition';

export interface Face {
  name: string;
  created_at: string;
  has_image: boolean;
  age?: number;
  gender?: string;
}

export interface FaceListResponse {
  faces: Face[];
  count: number;
}

export interface FaceDetection {
  bbox: number[];
  confidence: number;
  center: number[];
  area: number;
  age?: number;
  gender?: string;
}

export interface FaceRecognition extends FaceDetection {
  identity: string | null;
  similarity: number;
  is_known: boolean;
}

export interface DetectResponse {
  faces: FaceDetection[];
  count: number;
  inference_time_ms: number;
  device: string;
}

export interface RecognizeResponse {
  recognitions: FaceRecognition[];
  count: number;
  known_count: number;
  unknown_count: number;
  inference_time_ms: number;
  device: string;
  similarity_threshold: number;
}

export interface RegisterResponse {
  success: boolean;
  name: string;
  message: string;
  face_count: number;
  image_path: string;
}

export interface DeleteResponse {
  success: boolean;
  name: string;
  message: string;
  face_count: number;
}

export interface ServiceInfo {
  service: string;
  version: string;
  device: string;
  model_loaded: boolean;
  known_faces_count: number;
  similarity_threshold: number;
}

export interface HealthResponse {
  status: string;
  device: string;
  model_loaded: boolean;
  known_faces_count: number;
}

export interface ConfigResponse {
  similarity_threshold: number;
  faces_db_path: string;
  faces_images_path: string;
  known_faces_count: number;
}

class RecognitionApiError extends Error {
  constructor(
    public status: number,
    message: string
  ) {
    super(message);
    this.name = 'RecognitionApiError';
  }
}

async function handleResponse<T>(response: Response): Promise<T> {
  if (!response.ok) {
    const errorText = await response.text();
    let errorMessage = `Request failed with status ${response.status}`;
    try {
      const errorJson = JSON.parse(errorText);
      errorMessage = errorJson.detail || errorJson.message || errorMessage;
    } catch {
      if (errorText) {
        errorMessage = errorText;
      }
    }
    throw new RecognitionApiError(response.status, errorMessage);
  }

  const contentType = response.headers.get('content-type');
  if (contentType?.includes('application/json')) {
    return response.json();
  }
  return response.text() as unknown as T;
}

// Get service info
export async function getServiceInfo(): Promise<ServiceInfo> {
  const response = await fetch(`${RECOGNITION_BASE}/`);
  return handleResponse<ServiceInfo>(response);
}

// Health check
export async function getHealth(): Promise<HealthResponse> {
  const response = await fetch(`${RECOGNITION_BASE}/health`);
  return handleResponse<HealthResponse>(response);
}

// Get configuration
export async function getConfig(): Promise<ConfigResponse> {
  const response = await fetch(`${RECOGNITION_BASE}/config`);
  return handleResponse<ConfigResponse>(response);
}

// Set similarity threshold
export async function setThreshold(threshold: number): Promise<{ success: boolean; similarity_threshold: number }> {
  const response = await fetch(`${RECOGNITION_BASE}/config/threshold?threshold=${threshold}`, {
    method: 'POST',
  });
  return handleResponse(response);
}

// List registered faces
export async function listFaces(): Promise<FaceListResponse> {
  const response = await fetch(`${RECOGNITION_BASE}/faces`);
  return handleResponse<FaceListResponse>(response);
}

// Register a new face
export async function registerFace(name: string, imageFile: File): Promise<RegisterResponse> {
  const formData = new FormData();
  formData.append('name', name);
  formData.append('file', imageFile);

  const response = await fetch(`${RECOGNITION_BASE}/faces/register`, {
    method: 'POST',
    body: formData,
  });
  return handleResponse<RegisterResponse>(response);
}

// Delete a registered face
export async function deleteFace(name: string): Promise<DeleteResponse> {
  const response = await fetch(`${RECOGNITION_BASE}/faces/${encodeURIComponent(name)}`, {
    method: 'DELETE',
  });
  return handleResponse<DeleteResponse>(response);
}

// Get face image URL
export function getFaceImageUrl(name: string): string {
  return `${RECOGNITION_BASE}/faces/${encodeURIComponent(name)}/image`;
}

// Detect faces in an image
export async function detectFaces(imageFile: File): Promise<DetectResponse> {
  const formData = new FormData();
  formData.append('file', imageFile);

  const response = await fetch(`${RECOGNITION_BASE}/detect`, {
    method: 'POST',
    body: formData,
  });
  return handleResponse<DetectResponse>(response);
}

// Recognize faces in an image
export async function recognizeFaces(imageFile: File): Promise<RecognizeResponse> {
  const formData = new FormData();
  formData.append('file', imageFile);

  const response = await fetch(`${RECOGNITION_BASE}/recognize`, {
    method: 'POST',
    body: formData,
  });
  return handleResponse<RecognizeResponse>(response);
}

// Recognize faces and get annotated image as base64
export async function recognizeAnnotated(imageFile: File): Promise<{
  image: { data: string; content_type: string };
  recognitions: FaceRecognition[];
  count: number;
  known_count: number;
  unknown_count: number;
  inference_time_ms: number;
}> {
  const formData = new FormData();
  formData.append('file', imageFile);

  const response = await fetch(`${RECOGNITION_BASE}/recognize/annotated?format=base64`, {
    method: 'POST',
    body: formData,
  });
  return handleResponse(response);
}
