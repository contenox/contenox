import React from 'react';
import { AuthContext } from './authContext';
import type { AuthenticatedUser } from './types';

const localUser: AuthenticatedUser = {
  id: 'local-user',
  subject: 'local-user',
  email: 'local@localhost',
  friendlyName: 'Local user',
  username: 'local-user',
};

export const AuthProvider = ({ children }: { children: React.ReactNode }) => {
  return (
    <AuthContext.Provider value={{ user: localUser, isLoading: false, isError: false, error: null }}>
      {children}
    </AuthContext.Provider>
  );
};
