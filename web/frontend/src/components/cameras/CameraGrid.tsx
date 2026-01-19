import { useState, useEffect, useRef } from 'react';
import { Grid2X2, Grid3X3, Square, LayoutGrid, Rows2, VideoOff } from 'lucide-react';
import type { Camera, LayoutMode, MotionEvent } from '../../types';
import { Button } from '../ui';
import CameraFeed from './CameraFeed';
import CameraZoomModal from './CameraZoomModal';
import RecentDetections from './RecentDetections';

// Debug logging helper
const DEBUG = true;
const log = (...args: unknown[]) => {
  if (DEBUG) {
    console.log('[CameraGrid]', ...args);
  }
};

interface CameraGridProps {
  cameras: Camera[];
  events?: MotionEvent[];
  eventsLoading?: boolean;
  onViewEvent?: (event: MotionEvent) => void;
}

const LAYOUT_STORAGE_KEY = 'orbo-grid-layout';

const layoutOptions: { mode: LayoutMode; icon: typeof Square; label: string; cols: number; rows: number }[] = [
  { mode: 'single', icon: Square, label: '1x1', cols: 1, rows: 1 },
  { mode: 'dual', icon: Rows2, label: '2x1', cols: 2, rows: 1 },
  { mode: 'quad', icon: Grid2X2, label: '2x2', cols: 2, rows: 2 },
  { mode: 'six', icon: LayoutGrid, label: '3x2', cols: 3, rows: 2 },
  { mode: 'nine', icon: Grid3X3, label: '3x3', cols: 3, rows: 3 },
];

export default function CameraGrid({
  cameras,
  events = [],
  eventsLoading = false,
  onViewEvent,
}: CameraGridProps) {
  const [layout, setLayout] = useState<LayoutMode>(() => {
    const saved = localStorage.getItem(LAYOUT_STORAGE_KEY);
    return (saved as LayoutMode) || 'quad';
  });

  const [selectedCameras, setSelectedCameras] = useState<(string | null)[]>([]);
  const [zoomedCamera, setZoomedCamera] = useState<Camera | null>(null);
  const mountCountRef = useRef(0);

  const currentLayout = layoutOptions.find((l) => l.mode === layout) || layoutOptions[2];
  const maxSlots = currentLayout.cols * currentLayout.rows;

  // Log mount/unmount
  useEffect(() => {
    mountCountRef.current += 1;
    log('Component mounted (mount #' + mountCountRef.current + '), cameras:', cameras.length);
    return () => {
      log('Component unmounting (was mount #' + mountCountRef.current + ')');
    };
  }, []);

  // Initialize selected cameras when cameras load or layout changes
  useEffect(() => {
    const activeCameras = cameras.filter((c) => c.status === 'active');
    log('Updating selected cameras, active:', activeCameras.length, 'maxSlots:', maxSlots);
    const slots: (string | null)[] = [];

    for (let i = 0; i < maxSlots; i++) {
      if (activeCameras[i]) {
        slots.push(activeCameras[i].id);
      } else {
        slots.push(null);
      }
    }

    log('Selected camera IDs:', slots);
    setSelectedCameras(slots);
  }, [cameras, maxSlots]);

  const handleLayoutChange = (newLayout: LayoutMode) => {
    setLayout(newLayout);
    localStorage.setItem(LAYOUT_STORAGE_KEY, newLayout);
  };

  const getCameraById = (id: string | null): Camera | undefined => {
    if (!id) return undefined;
    return cameras.find((c) => c.id === id);
  };

  const getGridClasses = () => {
    switch (layout) {
      case 'single':
        return 'grid-cols-1 grid-rows-1';
      case 'dual':
        return 'grid-cols-2 grid-rows-1';
      case 'quad':
        return 'grid-cols-2 grid-rows-2';
      case 'six':
        return 'grid-cols-3 grid-rows-2';
      case 'nine':
        return 'grid-cols-3 grid-rows-3';
      default:
        return 'grid-cols-2 grid-rows-2';
    }
  };

  return (
    <>
      <div className="h-full flex">
        {/* Main grid area */}
        <div className="flex-1 flex flex-col min-w-0">
          {/* Layout selector toolbar */}
          <div className="flex items-center justify-between p-3 bg-bg-panel border-b border-border">
            <h2 className="text-sm font-semibold text-text-primary">Camera Grid</h2>
            <div className="flex items-center gap-1">
              {layoutOptions.map(({ mode, icon: Icon, label }) => (
                <Button
                  key={mode}
                  variant={layout === mode ? 'primary' : 'ghost'}
                  size="sm"
                  onClick={() => handleLayoutChange(mode)}
                  title={label}
                >
                  <Icon className="w-4 h-4" />
                </Button>
              ))}
            </div>
          </div>

          {/* Grid */}
          <div className={`flex-1 p-3 grid gap-2 ${getGridClasses()} auto-rows-fr overflow-hidden`}>
            {selectedCameras.slice(0, maxSlots).map((cameraId, index) => {
              const camera = getCameraById(cameraId);

              if (!camera) {
                return (
                  <div
                    key={index}
                    className="bg-bg-card rounded-lg border border-border border-dashed flex items-center justify-center text-text-muted"
                  >
                    <p className="text-sm">No camera</p>
                  </div>
                );
              }

              return (
                <div
                  key={camera.id}
                  className="relative group"
                >
                  <CameraFeed
                    camera={camera}
                    className="h-full"
                    onFullscreen={() => setZoomedCamera(camera)}
                  />
                </div>
              );
            })}
          </div>
        </div>

        {/* Recent detections sidebar */}
        <div className="w-64 flex-shrink-0">
          <RecentDetections
            events={events}
            isLoading={eventsLoading}
            onViewEvent={onViewEvent}
            maxItems={10}
          />
        </div>
      </div>

      {/* Zoom modal */}
      <CameraZoomModal camera={zoomedCamera} onClose={() => setZoomedCamera(null)} />
    </>
  );
}
