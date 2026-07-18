import { createContext } from 'react';
import { AuthenticatedUser } from './types';

export interface AuthContextType {
  /**
   * The (single, local) app user, present when access is granted — i.e. when
   * login is not required, or the browser holds a valid session cookie. Reading
   * components treat it as a truthy "is the app usable" flag; it is undefined
   * only while the remote-access gate is showing the login page.
   */
  user: AuthenticatedUser | undefined;
  isLoading: boolean;
  isError: boolean;
  error: Error | null;
  /** Whether the server requires remote-access login (a TOKEN is configured). */
  authRequired: boolean;
  /** Whether this browser is authenticated (session cookie accepted, or login not required). */
  authenticated: boolean;
  /** Re-query /ui/auth-status — call after a successful login or logout to swap the gate. */
  refresh: () => void;
}

export const AuthContext = createContext<AuthContextType>({
  user: undefined,
  isLoading: true,
  isError: false,
  error: null,
  authRequired: false,
  authenticated: true,
  refresh: () => {},
});
