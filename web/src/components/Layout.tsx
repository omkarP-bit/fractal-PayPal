import { useEffect, useState } from 'react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  Menu,
  Mail,
  ShieldCheck,
  BarChart3,
  CreditCard,
  Users,
  Settings,
  ChevronRight,
  Home,
  LogOut,
} from '../components/ui/Icons';
import { Button } from '../components/ui/Button';
import { useAuthStore } from '../store/auth';

const NAV_ITEMS = [
  { path: '/customer/cases', label: 'Cases', icon: Home, role: 'CUSTOMER' },
  { path: '/customer/cases/new', label: 'Report Issue', icon: Mail, role: 'CUSTOMER' },
  { path: '/merchant/cases', label: 'Cases', icon: Mail, role: 'MERCHANT' },
  { path: '/demo', label: 'Demo Controls', icon: Settings, role: 'MERCHANT' },
];

export function Layout({ children }: { children: React.ReactNode }) {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  // Manual role switch; null = follow the signed-in user.
  const [manualRole, setManualRole] = useState<'CUSTOMER' | 'MERCHANT' | null>(null);
  const [dark, setDark] = useState(() => document.documentElement.classList.contains('dark'));
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout } = useAuthStore();

  // Unauthenticated visitors always land on the login screen
  // (instead of a broken "Guest" dashboard).
  useEffect(() => {
    if (!user && location.pathname !== '/login') {
      navigate('/login', { replace: true });
    }
  }, [user, location.pathname, navigate]);

  // Sync nav with the signed-in user (merchant users see merchant links);
  // an explicit manual switch always takes precedence and is never undone.
  const role = manualRole ?? (user?.kind === 'MERCHANT_USER' ? 'MERCHANT' : 'CUSTOMER');

  // Merchant sees all tabs (customer + merchant) — original behavior.
  const visibleItems = NAV_ITEMS.filter((item) => item.role === role || item.role === 'CUSTOMER');
  // Highlight only the most specific match, so a child route
  // (e.g. /customer/cases/new) doesn't leave its parent (/customer/cases) lit too.
  const activePath = visibleItems
    .map((item) => item.path)
    .filter((path) => location.pathname === path || location.pathname.startsWith(`${path}/`))
    .sort((a, b) => b.length - a.length)[0];

  const toggleTheme = () => {
    const next = !dark;
    setDark(next);
    document.documentElement.classList.toggle('dark', next);
    try {
      localStorage.setItem('theme', next ? 'dark' : 'light');
    } catch {
      // ignore storage errors (e.g. private mode)
    }
  };

  return (
    <div className="min-h-screen bg-gray-50">
      {sidebarOpen && (
        <div className="fixed inset-0 bg-black/50 z-40" onClick={() => setSidebarOpen(false)} />
      )}

      <aside
        className={`fixed left-0 top-0 bottom-0 w-64 bg-white dark:bg-gray-800 border-r border-gray-200 z-50 transition-transform duration-300 md:translate-x-0 ${
          sidebarOpen ? 'translate-x-0' : '-translate-x-full'
        }`}
      >
        <div className="flex items-center justify-between h-16 px-6 border-b border-gray-200">
          <div className="flex items-center gap-3">
            <div className="bg-paypal-600 rounded-lg p-2 flex items-center justify-center">
              <span className="text-white font-bold text-xl tracking-tight">F</span>
            </div>
            <span className="font-bold text-gray-900 tracking-tight text-lg">FRACTAL</span>
          </div>
          <button onClick={() => setSidebarOpen(false)} className="text-gray-400 hover:text-gray-600 md:hidden">
            <span className="sr-only">Close menu</span>
            <div className="w-6 h-6">
              <svg fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth="2">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12"></path>
              </svg>
            </div>
          </button>
        </div>

        <nav className="p-4 space-y-1">
          {visibleItems.map((item) => (
            <Link
              key={item.path}
              to={item.path}
              className={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors ${
                activePath === item.path
                  ? 'bg-paypal-50 text-paypal-600 border-r-2 border-paypal-600 dark:text-paypal-300'
                  : 'text-gray-600 hover:bg-gray-50 hover:text-paypal-600 dark:hover:text-paypal-300'
              }`}
            >
              <item.icon className="w-4 h-4" />
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="mt-auto p-6 border-t border-gray-200">
          <button
            onClick={() => setManualRole(role === 'CUSTOMER' ? 'MERCHANT' : 'CUSTOMER')}
            className="w-full flex items-center justify-between px-4 py-2.5 rounded-lg border border-gray-200 text-sm font-medium hover:bg-gray-50 transition-colors"
          >
            <span>Switch to {role === 'CUSTOMER' ? 'Merchant' : 'Customer'}</span>
            <ChevronRight className="w-3 h-3" />
          </button>
          <button
            onClick={() => {
              logout();
              navigate('/login');
            }}
            className="w-full flex items-center justify-center px-4 py-2.5 rounded-lg border border-gray-200 text-sm font-medium text-red-600 hover:bg-red-50 mt-2 transition-colors"
          >
            Sign out
          </button>
        </div>
      </aside>

      <header className="sticky top-0 z-40 border-b border-gray-200 bg-white/95 dark:bg-gray-800/95 backdrop-blur-md md:ml-64">
        <div className="flex items-center justify-between px-6 py-3">
          <button
            onClick={() => setSidebarOpen(!sidebarOpen)}
            className="p-2 rounded-md text-gray-400 hover:bg-gray-100 md:hidden"
          >
            <span className="sr-only">Open menu</span>
            <Menu className="w-5 h-5" />
          </button>
          <div className="flex items-center gap-6">
            <div className="hidden md:flex flex-1 max-w-md">
              <input
                type="text"
                placeholder="Search cases..."
                className="w-full px-3 py-2 rounded-lg border border-gray-200 bg-gray-50 text-sm focus:ring-2 focus:ring-paypal-600 focus:border-paypal-600 transition-colors"
              />
            </div>
            <div className="flex items-center gap-3">
              <button
                type="button"
                onClick={toggleTheme}
                role="switch"
                aria-checked={dark}
                aria-label={dark ? 'Switch to light mode' : 'Switch to dark mode'}
                title="Toggle dark mode"
                className="relative flex h-8 w-[4.25rem] shrink-0 items-center rounded-full border border-gray-200 bg-gray-100 p-0.5 transition"
              >
                <span
                  className={`absolute inset-y-0.5 left-0.5 w-8 rounded-full bg-white shadow-sm transition-transform duration-300 ease-out dark:bg-gray-400 ${
                    dark ? 'translate-x-8' : 'translate-x-0'
                  }`}
                />
                <span
                  className={`relative z-10 flex w-8 justify-center text-sm leading-none transition-transform duration-500 ${
                    dark ? '-rotate-180 scale-75 opacity-50' : 'rotate-0 scale-100 opacity-100'
                  }`}
                >
                  ☀️
                </span>
                <span
                  className={`relative z-10 flex w-8 justify-center text-sm leading-none transition-transform duration-500 ${
                    dark ? 'rotate-0 scale-100 opacity-100' : 'rotate-180 scale-75 opacity-50'
                  }`}
                >
                  🌙
                </span>
              </button>
              <span className="hidden sm:block text-sm text-gray-500">{user?.kind ?? 'Guest'}</span>
              <div className="h-8 w-8 rounded-full bg-paypal-600 flex items-center justify-center text-white text-sm font-medium">
                {user?.kind?.[0]?.toUpperCase() ?? 'G'}
              </div>
            </div>
          </div>
        </div>
      </header>

      <main className={`pt-20 pb-8 px-4 sm:px-6 lg:px-8 md:ml-64 ${sidebarOpen ? 'ml-64' : ''}`}>
        {children ?? <Outlet />}
      </main>
    </div>
  );
}
