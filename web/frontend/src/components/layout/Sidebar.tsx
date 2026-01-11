import { Camera, Activity, Grid3X3, Settings, UserCircle } from 'lucide-react';

type TabId = 'cameras' | 'events' | 'grid' | 'faces' | 'settings';

interface SidebarProps {
  activeTab: TabId;
  onTabChange: (tab: TabId) => void;
}

const tabs = [
  { id: 'cameras' as TabId, label: 'Cameras', icon: Camera },
  { id: 'events' as TabId, label: 'Events', icon: Activity },
  { id: 'grid' as TabId, label: 'Grid View', icon: Grid3X3 },
  { id: 'faces' as TabId, label: 'Faces', icon: UserCircle },
  { id: 'settings' as TabId, label: 'Settings', icon: Settings },
];

export default function Sidebar({ activeTab, onTabChange }: SidebarProps) {
  return (
    <aside className="w-44 bg-bg-panel border-r border-border flex flex-col py-4 px-3 gap-1">
      {tabs.map(({ id, label, icon: Icon }) => (
        <button
          key={id}
          onClick={() => onTabChange(id)}
          className={`
            w-full px-3 py-2.5 rounded-lg flex items-center gap-3
            text-sm font-medium transition-colors
            ${
              activeTab === id
                ? 'bg-accent text-bg-dark'
                : 'text-text-secondary hover:text-text-primary hover:bg-bg-hover'
            }
          `}
        >
          <Icon className="w-5 h-5 flex-shrink-0" />
          <span>{label}</span>
        </button>
      ))}
    </aside>
  );
}
