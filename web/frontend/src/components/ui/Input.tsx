import { InputHTMLAttributes, forwardRef } from 'react';

interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  hint?: string;
}

const Input = forwardRef<HTMLInputElement, InputProps>(({ label, error, hint, className = '', id, ...props }, ref) => {
  const inputId = id || label?.toLowerCase().replace(/\s+/g, '-');

  return (
    <div className="w-full">
      {label && (
        <label htmlFor={inputId} className="block text-sm font-medium text-text-secondary mb-1">
          {label}
        </label>
      )}
      <input
        ref={ref}
        id={inputId}
        className={`
          w-full px-3 py-2
          bg-bg-card border border-border rounded-md
          text-text-primary placeholder-text-muted
          focus:outline-none focus:ring-2 focus:ring-accent focus:border-transparent
          disabled:opacity-50 disabled:cursor-not-allowed
          ${error ? 'border-accent-red focus:ring-accent-red' : ''}
          ${className}
        `}
        {...props}
      />
      {hint && !error && <p className="mt-1 text-xs text-text-muted">{hint}</p>}
      {error && <p className="mt-1 text-sm text-accent-red">{error}</p>}
    </div>
  );
});

Input.displayName = 'Input';

export default Input;
