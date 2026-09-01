import { useState } from 'react';
import { Film } from 'lucide-react';
import { cn, thumbnailUrl } from '@/lib/utils';

interface PageHeaderProps {
  title: React.ReactNode;
  description?: string;
  children?: React.ReactNode;
}

export function PageHeader({ title, description, children }: PageHeaderProps) {
  return (
    <div className="flex items-start justify-between mb-8">
      <div className="max-w-xl">
        <h1 className="text-2xl font-bold text-white tracking-tight">{title}</h1>
        {description && <p className="text-sm text-zinc-500 mt-1.5">{description}</p>}
      </div>
      {children && <div className="flex items-center gap-2 shrink-0">{children}</div>}
    </div>
  );
}

interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'danger' | 'ghost';
  size?: 'sm' | 'md';
}

export function Button({
  variant = 'primary',
  size = 'md',
  className,
  ...props
}: ButtonProps) {
  return (
    <button
      className={cn(
        'inline-flex items-center justify-center gap-1.5 rounded-md font-medium transition-colors disabled:opacity-40 disabled:pointer-events-none',
        size === 'sm' && 'px-2.5 py-1.5 text-xs',
        size === 'md' && 'px-3.5 py-2 text-sm',
        variant === 'primary' && 'bg-blue-600 text-white hover:bg-blue-500',
        variant === 'secondary' && 'bg-zinc-800 text-zinc-300 hover:bg-zinc-700 border border-zinc-700',
        variant === 'danger' && 'bg-red-600 text-white hover:bg-red-500',
        variant === 'ghost' && 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800',
        className,
      )}
      {...props}
    />
  );
}

interface BadgeProps {
  variant?: 'default' | 'success' | 'warning' | 'danger' | 'info';
  children: React.ReactNode;
  className?: string;
}

export function Badge({ variant = 'default', children, className }: BadgeProps) {
  return (
    <span
      className={cn(
        'inline-flex items-center px-2 py-0.5 rounded text-xs font-medium',
        variant === 'default' && 'bg-zinc-800 text-zinc-300',
        variant === 'success' && 'bg-emerald-900/50 text-emerald-400',
        variant === 'warning' && 'bg-amber-900/50 text-amber-400',
        variant === 'danger' && 'bg-red-900/50 text-red-400',
        variant === 'info' && 'bg-blue-900/50 text-blue-400',
        className,
      )}
    >
      {children}
    </span>
  );
}

interface CardProps {
  className?: string;
  children: React.ReactNode;
}

export function Card({ className, children }: CardProps) {
  return (
    <div className={cn('rounded-lg border border-zinc-800 bg-zinc-900/50 p-4', className)}>
      {children}
    </div>
  );
}

interface StatCardProps {
  label: string;
  value: number | string;
  icon?: React.ReactNode;
}

export function StatCard({ label, value, icon }: StatCardProps) {
  return (
    <div className="rounded-lg border border-zinc-800 bg-zinc-900/30 p-4">
      <div className="flex items-center justify-between">
        <div>
          <p className="text-xs text-zinc-500 font-medium uppercase tracking-wider">{label}</p>
          <p className="text-xl font-semibold text-white mt-0.5">{value}</p>
        </div>
        {icon && <div className="text-zinc-600">{icon}</div>}
      </div>
    </div>
  );
}

interface SpinnerProps {
  size?: 'sm' | 'md' | 'lg';
  className?: string;
}

export function Spinner({ size = 'md', className }: SpinnerProps) {
  return (
    <div
      className={cn(
        'flex items-center justify-center py-20',
        className
      )}
    >
      <div
        className={cn(
          'animate-spin rounded-full border-2 border-zinc-800 border-t-zinc-400',
          size === 'sm' && 'h-4 w-4',
          size === 'md' && 'h-6 w-6',
          size === 'lg' && 'h-8 w-8',
        )}
      />
    </div>
  );
}

export function EmptyState({ message }: { message: string }) {
  return (
    <div className="flex items-center justify-center py-16 text-zinc-500 text-sm">
      {message}
    </div>
  );
}

interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
}

export function Input({ label, className, ...props }: InputProps) {
  return (
    <div>
      {label && <label className="block text-xs text-zinc-500 font-medium mb-1">{label}</label>}
      <input
        className={cn(
          'w-full rounded-md border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 transition-colors',
          className,
        )}
        {...props}
      />
    </div>
  );
}

interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
}

export function Textarea({ label, className, ...props }: TextareaProps) {
  return (
    <div>
      {label && <label className="block text-xs text-zinc-500 font-medium mb-1">{label}</label>}
      <textarea
        className={cn(
          'w-full rounded-md border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 placeholder-zinc-500 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 transition-colors',
          className,
        )}
        {...props}
      />
    </div>
  );
}

interface SelectProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  options: { value: string; label: string }[];
}

export function Select({ label, options, className, ...props }: SelectProps) {
  return (
    <div>
      {label && <label className="block text-xs text-zinc-500 font-medium mb-1">{label}</label>}
      <select
        className={cn(
          'w-full rounded-md border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 focus:border-blue-500 focus:outline-none focus:ring-1 focus:ring-blue-500 transition-colors',
          className,
        )}
        {...props}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </div>
  );
}

interface TabsProps {
  value: string;
  onChange: (value: string) => void;
  tabs: { value: string; label: string; count?: number }[];
}

export function Tabs({ value, onChange, tabs }: TabsProps) {
  return (
    <div className="flex items-center gap-1 border-b border-zinc-800 overflow-x-auto">
      {tabs.map((t) => (
        <button
          key={t.value}
          onClick={() => onChange(t.value)}
          className={cn(
            'flex items-center gap-1.5 px-3 py-2 text-sm font-medium border-b-2 -mb-px transition-colors whitespace-nowrap',
            value === t.value
              ? 'text-white border-blue-500'
              : 'text-zinc-400 border-transparent hover:text-zinc-200 hover:border-zinc-700',
          )}
        >
          {t.label}
          {typeof t.count === 'number' && t.count > 0 && (
            <span
              className={cn(
                'text-[10px] px-1.5 py-0.5 rounded-full font-semibold leading-none',
                value === t.value
                  ? 'bg-blue-900/60 text-blue-300'
                  : 'bg-zinc-800 text-zinc-400',
              )}
            >
              {t.count}
            </span>
          )}
        </button>
      ))}
    </div>
  );
}

interface ConfirmDialogProps {
  open: boolean;
  title: string;
  message: string;
  confirmLabel?: string;
  onConfirm: () => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  open,
  title,
  message,
  confirmLabel = 'Delete',
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div className="bg-zinc-900 border border-zinc-700 rounded-lg p-6 max-w-sm w-full">
        <h3 className="text-base font-semibold text-white">{title}</h3>
        <p className="text-sm text-zinc-400 mt-2">{message}</p>
        <div className="flex justify-end gap-2 mt-4">
          <Button variant="secondary" size="sm" onClick={onCancel}>
            Cancel
          </Button>
          <Button variant="danger" size="sm" onClick={onConfirm}>
            {confirmLabel}
          </Button>
        </div>
      </div>
    </div>
  );
}

interface PaginationProps {
  page: number;
  totalPages: number;
  onPrev: () => void;
  onNext: () => void;
  hasMore?: boolean;
}

export function Pagination({ page, totalPages, onPrev, onNext, hasMore }: PaginationProps) {
  return (
    <div className="flex items-center justify-between mt-6 text-sm text-zinc-500">
      <span>Page {page} of {totalPages}</span>
      <div className="flex gap-2">
        <Button variant="ghost" size="sm" onClick={onPrev} disabled={page <= 1}>
          Previous
        </Button>
        <Button variant="ghost" size="sm" onClick={onNext} disabled={!hasMore}>
          Next
        </Button>
      </div>
    </div>
  );
}

interface VideoThumbProps {
  filename: string;
  alt?: string;
}

/** Video thumbnail with a film-icon fallback when the file is missing or the
 *  thumbnail hasn't been generated yet (avoids broken-image requests/404s). */
export function VideoThumb({ filename, alt = '' }: VideoThumbProps) {
  const [failed, setFailed] = useState(false);
  const src = filename ? thumbnailUrl(filename) : undefined;

  if (!src || failed) {
    return (
      <div className="w-full h-full bg-zinc-800 flex items-center justify-center text-zinc-600">
        <Film size={28} className="text-zinc-700" />
      </div>
    );
  }

  return (
    <img
      src={src}
      alt={alt}
      className="w-full h-full object-cover"
      loading="lazy"
      onError={() => setFailed(true)}
    />
  );
}
