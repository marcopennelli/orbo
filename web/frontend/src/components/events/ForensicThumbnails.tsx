import { useState, useEffect } from 'react';
import { Scan, Maximize2, X, ChevronLeft, ChevronRight } from 'lucide-react';
import { getForensicThumbnail, frameResponseToDataUrl } from '../../api/events';
import { Spinner } from '../ui';

interface ForensicThumbnailsProps {
  eventId: string;
  thumbnailCount: number;
}

interface ThumbnailState {
  loading: boolean;
  error: boolean;
  src: string | null;
}

export default function ForensicThumbnails({ eventId, thumbnailCount }: ForensicThumbnailsProps) {
  const [thumbnails, setThumbnails] = useState<ThumbnailState[]>([]);
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null);

  useEffect(() => {
    if (thumbnailCount === 0) return;

    // Initialize thumbnail states
    const initialState: ThumbnailState[] = Array(thumbnailCount).fill(null).map(() => ({
      loading: true,
      error: false,
      src: null,
    }));
    setThumbnails(initialState);

    // Load all thumbnails
    const loadThumbnails = async () => {
      for (let i = 0; i < thumbnailCount; i++) {
        try {
          const frame = await getForensicThumbnail(eventId, i);
          setThumbnails(prev => {
            const updated = [...prev];
            updated[i] = {
              loading: false,
              error: false,
              src: frameResponseToDataUrl(frame),
            };
            return updated;
          });
        } catch {
          setThumbnails(prev => {
            const updated = [...prev];
            updated[i] = {
              loading: false,
              error: true,
              src: null,
            };
            return updated;
          });
        }
      }
    };

    loadThumbnails();
  }, [eventId, thumbnailCount]);

  const handlePrevious = () => {
    if (selectedIndex === null || selectedIndex === 0) return;
    setSelectedIndex(selectedIndex - 1);
  };

  const handleNext = () => {
    if (selectedIndex === null || selectedIndex >= thumbnailCount - 1) return;
    setSelectedIndex(selectedIndex + 1);
  };

  // Handle keyboard navigation in fullscreen
  useEffect(() => {
    if (selectedIndex === null) return;

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setSelectedIndex(null);
      } else if (e.key === 'ArrowLeft') {
        handlePrevious();
      } else if (e.key === 'ArrowRight') {
        handleNext();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [selectedIndex, thumbnailCount]);

  if (thumbnailCount === 0) return null;

  return (
    <div className="p-3 bg-bg-card rounded-lg">
      <div className="flex items-center gap-2 mb-3">
        <Scan className="w-5 h-5 text-cyan-400" />
        <p className="text-sm font-medium text-text-primary">
          Forensic Face Analysis
        </p>
        <span className="text-xs text-text-muted">
          ({thumbnailCount} {thumbnailCount === 1 ? 'face' : 'faces'})
        </span>
      </div>

      <p className="text-xs text-text-muted mb-3">
        Biometric analysis
      </p>

      {/* Thumbnail grid */}
      <div className="grid grid-cols-2 sm:grid-cols-3 gap-2">
        {thumbnails.map((thumb, index) => (
          <div
            key={index}
            className="relative aspect-[3/4] bg-bg-dark rounded-lg overflow-hidden border border-border hover:border-cyan-500 transition-colors cursor-pointer group"
            onClick={() => !thumb.loading && !thumb.error && setSelectedIndex(index)}
          >
            {thumb.loading ? (
              <div className="absolute inset-0 flex items-center justify-center">
                <Spinner size="sm" />
              </div>
            ) : thumb.error ? (
              <div className="absolute inset-0 flex items-center justify-center text-text-muted">
                <Scan className="w-6 h-6 opacity-50" />
              </div>
            ) : thumb.src ? (
              <>
                <img
                  src={thumb.src}
                  alt={`Face ${index + 1}`}
                  className="w-full h-full object-cover"
                />
                <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                  <Maximize2 className="w-5 h-5 text-white" />
                </div>
                <div className="absolute top-1 left-1 px-1.5 py-0.5 bg-black/70 rounded text-xs text-cyan-400 font-mono">
                  #{index + 1}
                </div>
              </>
            ) : null}
          </div>
        ))}
      </div>

      {/* Fullscreen viewer */}
      {selectedIndex !== null && thumbnails[selectedIndex]?.src && (
        <div
          className="fixed inset-0 z-50 bg-black/95 flex items-center justify-center"
          onClick={() => setSelectedIndex(null)}
        >
          {/* Close button */}
          <button
            onClick={() => setSelectedIndex(null)}
            className="absolute top-4 right-4 p-2 bg-white/10 rounded-lg text-white hover:bg-white/20 transition-colors z-10"
            title="Close (Esc)"
          >
            <X className="w-6 h-6" />
          </button>

          {/* Navigation buttons */}
          {thumbnailCount > 1 && (
            <>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  handlePrevious();
                }}
                disabled={selectedIndex === 0}
                className="absolute left-4 top-1/2 -translate-y-1/2 p-3 bg-white/10 rounded-full text-white hover:bg-white/20 transition-colors disabled:opacity-30 disabled:cursor-not-allowed z-10"
                title="Previous (Left Arrow)"
              >
                <ChevronLeft className="w-8 h-8" />
              </button>
              <button
                onClick={(e) => {
                  e.stopPropagation();
                  handleNext();
                }}
                disabled={selectedIndex >= thumbnailCount - 1}
                className="absolute right-4 top-1/2 -translate-y-1/2 p-3 bg-white/10 rounded-full text-white hover:bg-white/20 transition-colors disabled:opacity-30 disabled:cursor-not-allowed z-10"
                title="Next (Right Arrow)"
              >
                <ChevronRight className="w-8 h-8" />
              </button>
            </>
          )}

          {/* Image */}
          <div className="relative max-w-[90vw] max-h-[90vh]" onClick={(e) => e.stopPropagation()}>
            <img
              src={thumbnails[selectedIndex].src!}
              alt={`Face ${selectedIndex + 1}`}
              className="max-w-full max-h-[85vh] object-contain rounded-lg border-2 border-cyan-500/50"
            />

            {/* Info bar */}
            <div className="absolute bottom-0 left-0 right-0 p-3 bg-gradient-to-t from-black/80 to-transparent rounded-b-lg">
              <div className="flex items-center justify-between text-white">
                <div className="flex items-center gap-2">
                  <Scan className="w-4 h-4 text-cyan-400" />
                  <span className="text-sm font-mono">SUBJECT #{selectedIndex + 1}</span>
                </div>
                <span className="text-xs text-gray-400">
                  {selectedIndex + 1} of {thumbnailCount}
                </span>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
