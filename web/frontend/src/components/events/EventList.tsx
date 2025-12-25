import { useState } from 'react';
import { RefreshCw } from 'lucide-react';
import type { MotionEvent, Camera } from '../../types';
import { Panel } from '../layout';
import { Button, Select, Spinner } from '../ui';
import EventCard from './EventCard';
import EventModal from './EventModal';

interface EventListProps {
  events: MotionEvent[];
  cameras: Camera[];
  isLoading?: boolean;
  onRefresh: () => void;
  selectedCameraId?: string;
  onCameraFilterChange: (cameraId: string) => void;
}

export default function EventList({
  events,
  cameras,
  isLoading,
  onRefresh,
  selectedCameraId,
  onCameraFilterChange,
}: EventListProps) {
  const [selectedEvent, setSelectedEvent] = useState<MotionEvent | null>(null);

  const cameraOptions = [
    { value: '', label: 'All Cameras' },
    ...cameras.map((c) => ({ value: c.id, label: c.name })),
  ];

  return (
    <>
      <Panel
        title="Motion Events"
        actions={
          <div className="flex items-center gap-2">
            <Select
              options={cameraOptions}
              value={selectedCameraId || ''}
              onChange={(e) => onCameraFilterChange(e.target.value)}
              className="w-40 text-xs"
            />
            <Button variant="ghost" size="sm" onClick={onRefresh} disabled={isLoading}>
              <RefreshCw className={`w-4 h-4 ${isLoading ? 'animate-spin' : ''}`} />
            </Button>
          </div>
        }
        className="h-full"
      >
        {isLoading && events.length === 0 ? (
          <div className="flex justify-center py-8">
            <Spinner />
          </div>
        ) : events.length === 0 ? (
          <div className="text-center py-8 text-text-muted">
            <p>No motion events recorded</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 max-h-[calc(100vh-250px)] overflow-y-auto">
            {events.map((event) => (
              <EventCard
                key={event.id}
                event={event}
                onView={() => setSelectedEvent(event)}
              />
            ))}
          </div>
        )}
      </Panel>

      <EventModal
        event={selectedEvent}
        isOpen={!!selectedEvent}
        onClose={() => setSelectedEvent(null)}
      />
    </>
  );
}
