import React from 'react';
import { RequestTableSkeleton } from './RequestTableSkeleton';

export interface RequestListSkeletonProps {
  count?: number;
}

export const RequestListSkeleton: React.FC<RequestListSkeletonProps> = ({ count }) => {
  return <RequestTableSkeleton rows={count} />;
};
