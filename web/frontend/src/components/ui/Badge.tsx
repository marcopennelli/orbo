import { ReactNode } from 'react';

interface BadgeProps {
  variant?: 'default' | 'success' | 'warning' | 'danger' | 'error' | 'info';
  children: ReactNode;
  className?: string;
}

export default function Badge({ variant = 'default', children, className = '' }: BadgeProps) {
  const variants = {
    default: 'bg-bg-card text-text-secondary border-border',
    success: 'bg-accent-green/20 text-accent-green border-accent-green/30',
    warning: 'bg-accent-orange/20 text-accent-orange border-accent-orange/30',
    danger: 'bg-accent-red/20 text-accent-red border-accent-red/30',
    error: 'bg-accent-red/20 text-accent-red border-accent-red/30',
    info: 'bg-accent/20 text-accent border-accent/30',
  };

  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 text-xs font-medium rounded-full border ${variants[variant]} ${className}`}
    >
      {children}
    </span>
  );
}
