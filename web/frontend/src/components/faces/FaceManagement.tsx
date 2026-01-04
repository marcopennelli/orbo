import { useState, useRef } from 'react';
import { UserPlus, Trash2, Upload, User, AlertCircle, CheckCircle, X, Camera } from 'lucide-react';
import { Button, Input, Modal, Spinner } from '../ui';
import * as recognitionApi from '../../api/recognition';
import type { Face, RecognizeResponse } from '../../api/recognition';

interface FaceManagementProps {
  faces: Face[];
  isLoading: boolean;
  onRefresh: () => void;
}

export default function FaceManagement({ faces, isLoading, onRefresh }: FaceManagementProps) {
  const [showRegisterModal, setShowRegisterModal] = useState(false);
  const [showTestModal, setShowTestModal] = useState(false);
  const [registerName, setRegisterName] = useState('');
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [previewUrl, setPreviewUrl] = useState<string | null>(null);
  const [isRegistering, setIsRegistering] = useState(false);
  const [isTesting, setIsTesting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null);
  const [isDeleting, setIsDeleting] = useState(false);
  const [testResult, setTestResult] = useState<RecognizeResponse | null>(null);
  const [testImageUrl, setTestImageUrl] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const testFileInputRef = useRef<HTMLInputElement>(null);

  const handleFileSelect = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (file) {
      setSelectedFile(file);
      setPreviewUrl(URL.createObjectURL(file));
      setError(null);
    }
  };

  const handleRegister = async () => {
    if (!registerName.trim()) {
      setError('Please enter a name');
      return;
    }
    if (!selectedFile) {
      setError('Please select an image');
      return;
    }

    setIsRegistering(true);
    setError(null);

    try {
      const result = await recognitionApi.registerFace(registerName.trim(), selectedFile);
      setSuccess(result.message);
      setShowRegisterModal(false);
      setRegisterName('');
      setSelectedFile(null);
      setPreviewUrl(null);
      onRefresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to register face');
    } finally {
      setIsRegistering(false);
    }
  };

  const handleDelete = async (name: string) => {
    setIsDeleting(true);
    try {
      await recognitionApi.deleteFace(name);
      setSuccess(`Face "${name}" deleted successfully`);
      setDeleteConfirm(null);
      onRefresh();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete face');
    } finally {
      setIsDeleting(false);
    }
  };

  const handleTestFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    setIsTesting(true);
    setTestResult(null);
    setTestImageUrl(URL.createObjectURL(file));
    setError(null);

    try {
      const result = await recognitionApi.recognizeFaces(file);
      setTestResult(result);
      setShowTestModal(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Recognition failed');
    } finally {
      setIsTesting(false);
    }
  };

  const closeRegisterModal = () => {
    setShowRegisterModal(false);
    setRegisterName('');
    setSelectedFile(null);
    setPreviewUrl(null);
    setError(null);
  };

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold text-text-primary">Face Recognition</h2>
          <p className="text-sm text-text-muted">
            {faces.length} registered {faces.length === 1 ? 'identity' : 'identities'}
          </p>
        </div>
        <div className="flex gap-2">
          <Button
            variant="secondary"
            onClick={() => testFileInputRef.current?.click()}
            disabled={isTesting}
          >
            {isTesting ? <Spinner size="sm" /> : <Camera className="w-4 h-4 mr-2" />}
            Test Recognition
          </Button>
          <Button onClick={() => setShowRegisterModal(true)}>
            <UserPlus className="w-4 h-4 mr-2" />
            Register Face
          </Button>
        </div>
        <input
          ref={testFileInputRef}
          type="file"
          accept="image/*"
          onChange={handleTestFile}
          className="hidden"
        />
      </div>

      {/* Status messages */}
      {error && (
        <div className="flex items-center gap-2 p-3 bg-accent-red/10 border border-accent-red/20 rounded-lg text-accent-red text-sm">
          <AlertCircle className="w-4 h-4 flex-shrink-0" />
          <span>{error}</span>
          <button onClick={() => setError(null)} className="ml-auto">
            <X className="w-4 h-4" />
          </button>
        </div>
      )}

      {success && (
        <div className="flex items-center gap-2 p-3 bg-accent-green/10 border border-accent-green/20 rounded-lg text-accent-green text-sm">
          <CheckCircle className="w-4 h-4 flex-shrink-0" />
          <span>{success}</span>
          <button onClick={() => setSuccess(null)} className="ml-auto">
            <X className="w-4 h-4" />
          </button>
        </div>
      )}

      {/* Faces Grid */}
      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : faces.length === 0 ? (
        <div className="text-center py-12 text-text-muted">
          <User className="w-12 h-12 mx-auto mb-3 opacity-50" />
          <p>No faces registered yet</p>
          <p className="text-sm mt-1">Click "Register Face" to add someone</p>
        </div>
      ) : (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4">
          {faces.map((face) => (
            <div
              key={face.name}
              className="bg-bg-secondary rounded-lg border border-border overflow-hidden group"
            >
              <div className="aspect-square bg-bg-tertiary relative">
                {face.has_image ? (
                  <img
                    src={recognitionApi.getFaceImageUrl(face.name)}
                    alt={face.name}
                    className="w-full h-full object-cover"
                  />
                ) : (
                  <div className="w-full h-full flex items-center justify-center">
                    <User className="w-12 h-12 text-text-muted" />
                  </div>
                )}
                {/* Delete overlay */}
                <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                  <button
                    onClick={() => setDeleteConfirm(face.name)}
                    className="p-2 bg-accent-red rounded-full text-white hover:bg-accent-red/80"
                  >
                    <Trash2 className="w-5 h-5" />
                  </button>
                </div>
              </div>
              <div className="p-3">
                <h3 className="font-medium text-text-primary truncate">{face.name}</h3>
                <div className="text-xs text-text-muted mt-1 space-y-0.5">
                  {face.age && <p>Age: ~{face.age}</p>}
                  {face.gender && <p>Gender: {face.gender}</p>}
                  {face.created_at && (
                    <p>{new Date(face.created_at).toLocaleDateString()}</p>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Register Modal */}
      <Modal isOpen={showRegisterModal} onClose={closeRegisterModal} title="Register New Face">
        <div className="space-y-4">
          <Input
            label="Name"
            value={registerName}
            onChange={(e) => setRegisterName(e.target.value)}
            placeholder="Enter person's name"
            disabled={isRegistering}
          />

          <div>
            <label className="block text-sm font-medium text-text-secondary mb-2">
              Photo
            </label>
            <div
              onClick={() => fileInputRef.current?.click()}
              className={`
                border-2 border-dashed rounded-lg p-6 text-center cursor-pointer
                transition-colors hover:border-accent-blue/50
                ${previewUrl ? 'border-accent-blue' : 'border-border'}
              `}
            >
              {previewUrl ? (
                <div className="relative">
                  <img
                    src={previewUrl}
                    alt="Preview"
                    className="max-h-48 mx-auto rounded-lg"
                  />
                  <button
                    onClick={(e) => {
                      e.stopPropagation();
                      setSelectedFile(null);
                      setPreviewUrl(null);
                    }}
                    className="absolute top-2 right-2 p-1 bg-bg-secondary rounded-full"
                  >
                    <X className="w-4 h-4" />
                  </button>
                </div>
              ) : (
                <>
                  <Upload className="w-8 h-8 mx-auto text-text-muted mb-2" />
                  <p className="text-sm text-text-muted">
                    Click to upload a photo with a single face
                  </p>
                </>
              )}
            </div>
            <input
              ref={fileInputRef}
              type="file"
              accept="image/*"
              onChange={handleFileSelect}
              className="hidden"
            />
          </div>

          {error && (
            <p className="text-sm text-accent-red">{error}</p>
          )}

          <div className="flex justify-end gap-3 pt-2">
            <Button variant="secondary" onClick={closeRegisterModal} disabled={isRegistering}>
              Cancel
            </Button>
            <Button onClick={handleRegister} loading={isRegistering}>
              Register
            </Button>
          </div>
        </div>
      </Modal>

      {/* Delete Confirmation Modal */}
      <Modal
        isOpen={!!deleteConfirm}
        onClose={() => setDeleteConfirm(null)}
        title="Delete Face"
      >
        <div className="space-y-4">
          <p className="text-text-secondary">
            Are you sure you want to delete <strong>{deleteConfirm}</strong>? This action cannot be undone.
          </p>
          <div className="flex justify-end gap-3">
            <Button variant="secondary" onClick={() => setDeleteConfirm(null)} disabled={isDeleting}>
              Cancel
            </Button>
            <Button
              variant="danger"
              onClick={() => deleteConfirm && handleDelete(deleteConfirm)}
              loading={isDeleting}
            >
              Delete
            </Button>
          </div>
        </div>
      </Modal>

      {/* Test Results Modal */}
      <Modal
        isOpen={showTestModal}
        onClose={() => {
          setShowTestModal(false);
          setTestResult(null);
          setTestImageUrl(null);
        }}
        title="Recognition Results"
      >
        <div className="space-y-4">
          {testImageUrl && (
            <img
              src={testImageUrl}
              alt="Test"
              className="max-h-64 mx-auto rounded-lg"
            />
          )}

          {testResult && (
            <div className="space-y-3">
              <div className="flex justify-between text-sm">
                <span className="text-text-muted">Faces detected:</span>
                <span className="text-text-primary font-medium">{testResult.count}</span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-text-muted">Known faces:</span>
                <span className="text-accent-green font-medium">{testResult.known_count}</span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-text-muted">Unknown faces:</span>
                <span className="text-accent-yellow font-medium">{testResult.unknown_count}</span>
              </div>
              <div className="flex justify-between text-sm">
                <span className="text-text-muted">Inference time:</span>
                <span className="text-text-primary">{testResult.inference_time_ms.toFixed(0)}ms</span>
              </div>

              {testResult.recognitions.length > 0 && (
                <div className="border-t border-border pt-3 mt-3">
                  <h4 className="text-sm font-medium text-text-primary mb-2">Detected Faces:</h4>
                  <div className="space-y-2">
                    {testResult.recognitions.map((rec, idx) => (
                      <div
                        key={idx}
                        className={`p-2 rounded-lg text-sm ${
                          rec.is_known
                            ? 'bg-accent-green/10 border border-accent-green/20'
                            : 'bg-accent-yellow/10 border border-accent-yellow/20'
                        }`}
                      >
                        <div className="flex justify-between items-center">
                          <span className={rec.is_known ? 'text-accent-green' : 'text-accent-yellow'}>
                            {rec.is_known ? rec.identity : 'Unknown'}
                          </span>
                          {rec.is_known && (
                            <span className="text-xs text-text-muted">
                              {(rec.similarity * 100).toFixed(0)}% match
                            </span>
                          )}
                        </div>
                        {rec.age && rec.gender && (
                          <p className="text-xs text-text-muted mt-1">
                            {rec.gender}, ~{rec.age} years
                          </p>
                        )}
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          )}

          <div className="flex justify-end pt-2">
            <Button
              variant="secondary"
              onClick={() => {
                setShowTestModal(false);
                setTestResult(null);
                setTestImageUrl(null);
              }}
            >
              Close
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}
