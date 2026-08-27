import type { HTMLAttributes } from 'react';

export function Skeleton({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={`animate-pulse bg-[var(--border-color)] rounded-sm ${className || ''}`}
      {...props}
    />
  );
}

export function SkeletonRow() {
  return (
    <div className="flex items-center gap-3 px-3 py-2.5 border-b border-[var(--border-color)]">
      <Skeleton className="h-3.5 w-3.5" />
      <Skeleton className="h-3 w-40" />
      <Skeleton className="h-4 w-12 rounded-sm ml-auto" />
    </div>
  );
}
