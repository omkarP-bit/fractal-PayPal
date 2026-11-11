import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export interface User {
  sub: string;
  kind: 'CUSTOMER' | 'MERCHANT_USER' | 'PLATFORM_ADMIN' | 'SERVICE';
  merchant_id?: string;
  merchant_name?: string;
  role?: string;
  permissions?: string[];
  email?: string;
  display_name?: string;
}

interface AuthState {
  token: string | null;
  user: User | null;
  isAuthenticated: boolean;
  login: (token: string, user: User) => void;
  logout: () => void;
  setToken: (token: string) => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      token: null,
      user: null,
      isAuthenticated: false,
      login: (token, user) => {
        set({ token, user, isAuthenticated: true });
      },
      logout: () => {
        localStorage.removeItem('fractal_token');
        localStorage.removeItem('fractal_auth');
        set({ token: null, user: null, isAuthenticated: false });
      },
      setToken: (token) => {
        set({ token });
      },
    }),
    {
      name: 'fractal_auth',
    }
  )
);
