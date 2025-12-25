import { Camera, Activity, Grid3X3, Settings } from 'lucide-react';

type TabId = 'cameras' | 'events' | 'grid' | 'settings';

interface SidebarProps {
  activeTab: TabId;
  onTabChange: (tab: TabId) => void;
}

const tabs = [
  { id: 'cameras' as TabId, label: 'Cameras', icon: Camera },
  { id: 'events' as TabId, label: 'Events', icon: Activity },
  { id: 'grid' as TabId, label: 'Grid View', icon: Grid3X3 },
  { id: 'settings' as TabId, label: 'Settings', icon: Settings },
];

export default function Sidebar({ activeTab, onTabChange }: SidebarProps) {
  return (
    <aside className="w-16 bg-bg-panel border-r border-border flex flex-col items-center py-4 gap-2">
      {tabs.map(({ id, label, icon: Icon }) => (
        <button
          key={id}
          onClick={() => onTabChange(id)}
          title={label}
          className={`
            w-12 h-12 rounded-lg flex items-center justify-center
            transition-colors
            ${
              activeTab === id
                ? 'bg-accent text-bg-dark'
                : 'text-text-secondary hover:text-text-primary hover:bg-bg-hover'
            }
          `}
        >
          <Icon className="w-5 h-5" />
        </button>
      ))}
    </aside>
  );
}
