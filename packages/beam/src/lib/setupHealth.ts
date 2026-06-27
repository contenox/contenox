import type { SetupIssue, SetupStatus } from './types';

export function getBlockingSetupIssue(status?: SetupStatus | null): SetupIssue | null {
  if (!status) return null;
  return status.issues.find(issue => issue.severity === 'error') ?? null;
}

export function getSetupIssueFixPath(issue?: SetupIssue | null): string {
  if (!issue?.fixPath) return '/settings';
  return issue.fixPath.startsWith('/') ? issue.fixPath : `/${issue.fixPath}`;
}

export function hasBlockingSetupIssue(status?: SetupStatus | null): boolean {
  return getBlockingSetupIssue(status) != null;
}
