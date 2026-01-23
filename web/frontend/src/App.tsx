import { useState, useCallback, useEffect } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Header, Sidebar } from './components/layout';
import { CameraList, CameraForm, CameraFeed, CameraGrid } from './components/cameras';
import { EventList, EventModal } from './components/events';
import { SettingsPanel } from './components/settings';
import { FaceManagement } from './components/faces';
import { LoginForm } from './components/auth';
import type { MotionEvent } from './types';
import * as recognitionApi from './api/recognition';
import type { Face } from './api/recognition';
import {
  useCameras,
  useCreateCamera,
  useUpdateCamera,
  useDeleteCamera,
  useActivateCamera,
  useDeactivateCamera,
  useSetEventsEnabled,
  useSetNotificationsEnabled,
} from './hooks/useCameras';
import { useInfiniteEvents } from './hooks/useEvents';
import {
  useSystemStatus,
  useStartDetection,
  useStopDetection,
} from './hooks/useSystemStatus';
import {
  useTelegramConfig,
  useUpdateTelegramConfig,
  useTestTelegram,
  useYoloConfig,
  useUpdateYoloConfig,
  useTestYolo,
  usePipelineConfig,
  useUpdatePipelineConfig,
  useRecognitionConfig,
  useUpdateRecognitionConfig,
  useTestRecognition,
} from './hooks/useConfig';
import { useAuth } from './hooks/useAuth';
import type { Camera, CameraCreatePayload, CameraUpdatePayload } from './types';

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      staleTime: 5000,
    },
  },
});

type TabId = 'cameras' | 'events' | 'grid' | 'faces' | 'settings';

function AppContent() {
  const [activeTab, setActiveTab] = useState<TabId>('cameras');
  const [selectedCamera, setSelectedCamera] = useState<Camera | null>(null);
  const [isFormOpen, setIsFormOpen] = useState(false);
  const [editingCamera, setEditingCamera] = useState<Camera | undefined>();
  const [isSettingsOpen, setIsSettingsOpen] = useState(false);
  const [eventCameraFilter, setEventCameraFilter] = useState<string>('');
  const [loadingCameraId, setLoadingCameraId] = useState<string | undefined>();
  const [selectedEvent, setSelectedEvent] = useState<MotionEvent | null>(null);

  // Face recognition state
  const [faces, setFaces] = useState<Face[]>([]);
  const [facesLoading, setFacesLoading] = useState(false);

  // Auth state
  const { isAuthenticated, authEnabled, isLoading: authLoading, logout } = useAuth();

  // Queries
  const { data: cameras = [], isLoading: camerasLoading } = useCameras();
  const {
    data: eventsData,
    isLoading: eventsLoading,
    isFetchingNextPage,
    hasNextPage,
    fetchNextPage,
    refetch: refetchEvents,
  } = useInfiniteEvents(eventCameraFilter || undefined);

  // Flatten paginated events data
  const events = eventsData?.pages.flat() ?? [];

  // Keep selectedCamera in sync with the latest camera data from the query
  // This ensures status changes (active/inactive) and other property updates are reflected in the CameraFeed
  useEffect(() => {
    if (selectedCamera) {
      const updatedCamera = cameras.find(c => c.id === selectedCamera.id);
      if (updatedCamera) {
        // Compare key properties that affect the UI
        const needsUpdate =
          updatedCamera.status !== selectedCamera.status ||
          updatedCamera.events_enabled !== selectedCamera.events_enabled ||
          updatedCamera.notifications_enabled !== selectedCamera.notifications_enabled ||
          updatedCamera.name !== selectedCamera.name;

        if (needsUpdate) {
          setSelectedCamera(updatedCamera);
        }
      }
    }
  }, [cameras, selectedCamera]);
  const { data: systemStatus } = useSystemStatus();
  const { data: telegramConfig } = useTelegramConfig();
  const { data: yoloConfig } = useYoloConfig();
  const { data: pipelineConfig } = usePipelineConfig();
  const { data: recognitionConfig } = useRecognitionConfig();

  // Mutations
  const createCamera = useCreateCamera();
  const updateCamera = useUpdateCamera();
  const deleteCamera = useDeleteCamera();
  const activateCamera = useActivateCamera();
  const deactivateCamera = useDeactivateCamera();
  const setEventsEnabled = useSetEventsEnabled();
  const setNotificationsEnabled = useSetNotificationsEnabled();
  const startDetection = useStartDetection();
  const stopDetection = useStopDetection();
  const updateTelegram = useUpdateTelegramConfig();
  const testTelegram = useTestTelegram();
  const updateYolo = useUpdateYoloConfig();
  const testYolo = useTestYolo();
  const updatePipeline = useUpdatePipelineConfig();
  const updateRecognition = useUpdateRecognitionConfig();
  const testRecognition = useTestRecognition();

  // Handlers
  const handleAddCamera = useCallback(() => {
    setEditingCamera(undefined);
    setIsFormOpen(true);
  }, []);

  const handleEditCamera = useCallback((camera: Camera) => {
    setEditingCamera(camera);
    setIsFormOpen(true);
  }, []);

  const handleFormSubmit = useCallback(
    async (data: CameraCreatePayload | CameraUpdatePayload) => {
      if (editingCamera) {
        await updateCamera.mutateAsync({ id: editingCamera.id, data });
      } else {
        await createCamera.mutateAsync(data as CameraCreatePayload);
      }
      setIsFormOpen(false);
      setEditingCamera(undefined);
    },
    [editingCamera, createCamera, updateCamera]
  );

  const handleDeleteCamera = useCallback(
    async (camera: Camera) => {
      if (window.confirm(`Delete camera "${camera.name}"?`)) {
        await deleteCamera.mutateAsync(camera.id);
        if (selectedCamera?.id === camera.id) {
          setSelectedCamera(null);
        }
      }
    },
    [deleteCamera, selectedCamera]
  );

  const handleToggleCameraActive = useCallback(
    async (camera: Camera) => {
      setLoadingCameraId(camera.id);
      try {
        let updatedCamera: Camera;
        if (camera.status === 'active') {
          updatedCamera = await deactivateCamera.mutateAsync(camera.id);
        } else {
          updatedCamera = await activateCamera.mutateAsync(camera.id);
        }
        // Immediately update selectedCamera if it's the same camera
        // This ensures the stream connects right away without waiting for query refetch
        if (selectedCamera?.id === camera.id) {
          setSelectedCamera(updatedCamera);
        }
      } finally {
        setLoadingCameraId(undefined);
      }
    },
    [activateCamera, deactivateCamera, selectedCamera]
  );

  const handleToggleCameraEvents = useCallback(
    async (camera: Camera) => {
      setLoadingCameraId(camera.id);
      try {
        const updatedCamera = await setEventsEnabled.mutateAsync({
          id: camera.id,
          enabled: !camera.events_enabled,
        });
        // Immediately update selectedCamera if it's the same camera
        if (selectedCamera?.id === camera.id) {
          setSelectedCamera(updatedCamera);
        }
      } finally {
        setLoadingCameraId(undefined);
      }
    },
    [setEventsEnabled, selectedCamera]
  );

  const handleToggleCameraNotifications = useCallback(
    async (camera: Camera) => {
      setLoadingCameraId(camera.id);
      try {
        const updatedCamera = await setNotificationsEnabled.mutateAsync({
          id: camera.id,
          enabled: !camera.notifications_enabled,
        });
        // Immediately update selectedCamera if it's the same camera
        if (selectedCamera?.id === camera.id) {
          setSelectedCamera(updatedCamera);
        }
      } finally {
        setLoadingCameraId(undefined);
      }
    },
    [setNotificationsEnabled, selectedCamera]
  );

  const handleToggleDetection = useCallback(async () => {
    if (systemStatus?.motion_detection_active) {
      await stopDetection.mutateAsync();
    } else {
      await startDetection.mutateAsync();
    }
  }, [systemStatus, startDetection, stopDetection]);

  const handleTabChange = useCallback((tab: TabId) => {
    if (tab === 'settings') {
      setIsSettingsOpen(true);
    } else {
      setActiveTab(tab);
    }
  }, []);

  // Load faces when faces tab is active
  const loadFaces = useCallback(async () => {
    setFacesLoading(true);
    try {
      const response = await recognitionApi.listFaces();
      setFaces(response.faces);
    } catch (error) {
      console.error('Failed to load faces:', error);
      setFaces([]);
    } finally {
      setFacesLoading(false);
    }
  }, []);

  useEffect(() => {
    if (activeTab === 'faces') {
      loadFaces();
    }
  }, [activeTab, loadFaces]);

  // Default configs for settings panel
  const defaultTelegramConfig = telegramConfig || {
    telegram_enabled: false,
    cooldown_seconds: 30,
  };

  const defaultYoloConfig = yoloConfig || {
    enabled: false,
    service_endpoint: 'http://yolo-service:8081',
    confidence_threshold: 0.5,
    security_mode: true,
    draw_boxes: false,
    classes_filter: '',
  };

  const defaultPipelineConfig = pipelineConfig || {
    mode: 'motion_triggered' as const,
    execution_mode: 'sequential' as const,
    detectors: [],
    schedule_interval: '5s',
    motion_sensitivity: 0.1,
    motion_cooldown_seconds: 2,
  };

  const defaultRecognitionConfig = recognitionConfig || {
    enabled: false,
    service_endpoint: 'recognition-service:50052',
    similarity_threshold: 0.5,
    known_face_color: '#00FF00',
    unknown_face_color: '#FF0000',
    box_thickness: 2,
  };

  // Compute camera counts from system status
  const activeCameras = systemStatus?.cameras?.filter(c => c.status === 'active').length ?? 0;
  const totalCameras = systemStatus?.cameras?.length ?? cameras.length;

  // Show loading spinner while checking auth status
  if (authLoading) {
    return (
      <div className="h-screen flex items-center justify-center bg-bg-dark">
        <div className="animate-spin rounded-full h-12 w-12 border-4 border-accent border-t-transparent"></div>
      </div>
    );
  }

  // Show login form if auth is enabled and user is not authenticated
  if (authEnabled && !isAuthenticated) {
    return <LoginForm onSuccess={() => window.location.reload()} />;
  }

  return (
    <div className="h-screen flex flex-col bg-bg-dark">
      <Header
        detectionRunning={systemStatus?.motion_detection_active ?? false}
        activeCameras={activeCameras}
        totalCameras={totalCameras}
        yoloEnabled={yoloConfig?.enabled ?? false}
        telegramEnabled={telegramConfig?.telegram_enabled ?? false}
        onToggleDetection={handleToggleDetection}
        onOpenSettings={() => setIsSettingsOpen(true)}
        isLoading={startDetection.isPending || stopDetection.isPending}
        isAuthEnabled={authEnabled}
        onLogout={logout}
      />

      <div className="flex-1 flex overflow-hidden">
        <Sidebar activeTab={activeTab} onTabChange={handleTabChange} />

        <main className="flex-1 overflow-hidden p-6">
          {activeTab === 'cameras' && (
            <div className="h-full flex gap-6">
              <div className="w-80 flex-shrink-0">
                <CameraList
                  cameras={cameras}
                  selectedCamera={selectedCamera}
                  onSelectCamera={setSelectedCamera}
                  onAddCamera={handleAddCamera}
                  onEditCamera={handleEditCamera}
                  onDeleteCamera={handleDeleteCamera}
                  onToggleCameraActive={handleToggleCameraActive}
                  onToggleCameraEvents={handleToggleCameraEvents}
                  onToggleCameraNotifications={handleToggleCameraNotifications}
                  isLoading={camerasLoading}
                  loadingCameraId={loadingCameraId}
                />
              </div>
              <div className="flex-1 bg-bg-panel rounded-lg border border-border overflow-hidden">
                {selectedCamera ? (
                  <CameraFeed
                    camera={selectedCamera}
                    className="h-full"
                    rawMode={true} // Cameras tab shows raw stream without annotations
                  />
                ) : (
                  <div className="h-full flex items-center justify-center text-text-muted">
                    <p>Select a camera to view feed</p>
                  </div>
                )}
              </div>
            </div>
          )}

          {activeTab === 'events' && (
            <EventList
              events={events}
              cameras={cameras}
              isLoading={eventsLoading}
              isFetchingNextPage={isFetchingNextPage}
              hasNextPage={hasNextPage}
              onRefresh={() => refetchEvents()}
              onLoadMore={() => fetchNextPage()}
              selectedCameraId={eventCameraFilter}
              onCameraFilterChange={setEventCameraFilter}
            />
          )}

          {activeTab === 'grid' && (
            <div className="h-full bg-bg-panel rounded-lg border border-border overflow-hidden">
              <CameraGrid
                cameras={cameras}
                events={events}
                eventsLoading={eventsLoading}
                onViewEvent={(event) => setSelectedEvent(event)}
              />
            </div>
          )}

          {activeTab === 'faces' && (
            <div className="h-full bg-bg-panel rounded-lg border border-border overflow-auto p-4">
              <FaceManagement
                faces={faces}
                isLoading={facesLoading}
                onRefresh={loadFaces}
              />
            </div>
          )}
        </main>
      </div>

      {/* Modals */}
      <CameraForm
        isOpen={isFormOpen}
        onClose={() => {
          setIsFormOpen(false);
          setEditingCamera(undefined);
        }}
        onSubmit={handleFormSubmit}
        camera={editingCamera}
        isLoading={createCamera.isPending || updateCamera.isPending}
      />

      <SettingsPanel
        isOpen={isSettingsOpen}
        onClose={() => setIsSettingsOpen(false)}
        telegramConfig={defaultTelegramConfig}
        yoloConfig={defaultYoloConfig}
        pipelineConfig={defaultPipelineConfig}
        recognitionConfig={defaultRecognitionConfig}
        onSaveTelegram={(config) => updateTelegram.mutate(config)}
        onSaveYolo={(config) => updateYolo.mutate(config)}
        onSavePipeline={(config) => updatePipeline.mutate(config)}
        onSaveRecognition={(config) => updateRecognition.mutate(config)}
        onTestTelegram={() => testTelegram.mutateAsync()}
        onTestYolo={() => testYolo.mutateAsync()}
        onTestRecognition={() => testRecognition.mutateAsync()}
        isSavingTelegram={updateTelegram.isPending}
        isSavingYolo={updateYolo.isPending}
        isSavingPipeline={updatePipeline.isPending}
        isSavingRecognition={updateRecognition.isPending}
      />

      <EventModal
        event={selectedEvent}
        cameras={cameras}
        isOpen={!!selectedEvent}
        onClose={() => setSelectedEvent(null)}
      />
    </div>
  );
}

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <AppContent />
    </QueryClientProvider>
  );
}
