import React from 'react';

export function Skeleton({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={`animate-pulse bg-gray-200 dark:bg-slate-700/50 rounded-md ${className || ''}`}
      {...props}
    />
  );
}

export function SkeletonRow() {
  return (
    <div className="flex items-center gap-4 px-6 py-4 border-b border-gray-100 dark:border-slate-800">
      <Skeleton className="h-5 w-5 rounded" />
      <Skeleton className="h-4 w-48 rounded" />
      <Skeleton className="h-5 w-16 rounded-full ml-auto" />
      <Skeleton className="h-8 w-8 rounded-full" />
    </div>
  );
}
