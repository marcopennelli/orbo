import { ReactNode } from 'react';

interface PanelProps {
  title: string;
  children: ReactNode;
  actions?: ReactNode;
  className?: string;
}

export default function Panel({ title, children, actions, className = '' }: PanelProps) {
  return (
    <div className={`bg-bg-panel border border-border rounded-lg overflow-hidden ${className}`}>
      <div className="flex items-center justify-between px-4 py-3 border-b border-border">
        <h2 className="text-sm font-semibold text-text-primary">{title}</h2>
        {actions && <div className="flex items-center gap-2">{actions}</div>}
      </div>
      <div className="p-4">{children}</div>
    </div>
  );
}
