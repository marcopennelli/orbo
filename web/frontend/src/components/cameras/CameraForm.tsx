import { useState, useEffect } from 'react';
import { Bell, BellOff } from 'lucide-react';
import type { Camera, CameraCreatePayload, CameraUpdatePayload } from '../../types';
import { Button, Input, Modal, Switch } from '../ui';

interface CameraFormProps {
  isOpen: boolean;
  onClose: () => void;
  onSubmit: (data: CameraCreatePayload | CameraUpdatePayload) => void;
  camera?: Camera;
  isLoading?: boolean;
}

export default function CameraForm({ isOpen, onClose, onSubmit, camera, isLoading }: CameraFormProps) {
  const [name, setName] = useState('');
  const [device, setDevice] = useState('');
  const [resolution, setResolution] = useState('640x480');
  const [fps, setFps] = useState(30);
  const [alertsEnabled, setAlertsEnabled] = useState(true);
  const [errors, setErrors] = useState<{ name?: string; device?: string }>({});

  const isEditing = !!camera;
  const isActive = camera?.status === 'active';

  useEffect(() => {
    if (camera) {
      setName(camera.name);
      setDevice(camera.device);
      setResolution(camera.resolution || '640x480');
      setFps(camera.fps || 30);
      setAlertsEnabled(camera.alerts_enabled);
    } else {
      setName('');
      setDevice('');
      setResolution('640x480');
      setFps(30);
      setAlertsEnabled(true);
    }
    setErrors({});
  }, [camera, isOpen]);

  const validate = (): boolean => {
    const newErrors: { name?: string; device?: string } = {};

    if (!name.trim()) {
      newErrors.name = 'Name is required';
    }

    if (!device.trim()) {
      newErrors.device = 'Device path is required';
    }

    setErrors(newErrors);
    return Object.keys(newErrors).length === 0;
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!validate()) return;

    if (isEditing) {
      // Include device if it was changed and camera is inactive
      const payload: CameraUpdatePayload = {
        name: name.trim(),
        resolution,
        fps,
        alerts_enabled: alertsEnabled,
      };
      // Only include device if changed
      if (device.trim() !== camera?.device) {
        payload.device = device.trim();
      }
      onSubmit(payload);
    } else {
      onSubmit({
        name: name.trim(),
        device: device.trim(),
        resolution,
        fps,
        alerts_enabled: alertsEnabled,
      } as CameraCreatePayload);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={isEditing ? 'Edit Camera' : 'Add Camera'}>
      <form onSubmit={handleSubmit} className="space-y-4">
        <Input
          label="Camera Name"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Living Room Camera"
          error={errors.name}
        />

        <Input
          label="Device Path / URL"
          value={device}
          onChange={(e) => setDevice(e.target.value)}
          placeholder="/dev/video0 or http://..."
          error={errors.device}
          disabled={isEditing && isActive}
          hint={
            isEditing && isActive
              ? 'Deactivate the camera first to change the device/URL'
              : 'USB: /dev/video0, HTTP: http://..., RTSP: rtsp://...'
          }
        />

        <Input
          label="Resolution"
          value={resolution}
          onChange={(e) => setResolution(e.target.value)}
          placeholder="640x480"
        />

        <Input
          label="FPS"
          type="number"
          value={fps}
          onChange={(e) => setFps(parseInt(e.target.value) || 30)}
          min={1}
          max={60}
        />

        {/* Alerts Toggle */}
        <div className="flex items-center justify-between p-3 rounded-lg border border-border bg-bg-card/50">
          <div className="flex items-center gap-3">
            {alertsEnabled ? (
              <Bell size={18} className="text-accent" />
            ) : (
              <BellOff size={18} className="text-text-muted" />
            )}
            <div>
              <span className="text-sm text-text-secondary block">Enable Alerts</span>
              <span className="text-xs text-text-muted">
                {alertsEnabled
                  ? 'Events will be created and notifications sent'
                  : 'Detection runs for display only, no alerts'}
              </span>
            </div>
          </div>
          <Switch
            checked={alertsEnabled}
            onChange={setAlertsEnabled}
          />
        </div>

        <div className="flex justify-end gap-3 pt-4 border-t border-border">
          <Button variant="secondary" type="button" onClick={onClose}>
            Cancel
          </Button>
          <Button type="submit" loading={isLoading}>
            {isEditing ? 'Save Changes' : 'Add Camera'}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
