import { useState } from 'react';
import { Link, useLocation } from 'react-router-dom';
import {
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
  { path: '/merchant/cases/new', label: 'New Case', icon: Mail, role: 'MERCHANT' },
  { path: '/demo', label: 'Demo Controls', icon: Settings, role: 'MERCHANT' },
];

export function Layout({ children }: { children: React.ReactNode }) {
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [role, setRole] = useState<'CUSTOMER' | 'MERCHANT' | null>(null);
  const location = useLocation();
  const { user, logout } = useAuthStore();

  const isActive = (path: string) => location.pathname === path || location.pathname.startsWith(path + '/');

  return (
    <div className="min-h-screen bg-gray-50">
      {sidebarOpen && (
        <div className="fixed inset-0 bg-black/50 z-40" onClick={() => setSidebarOpen(false)} />
      )}

      <aside
        className={`fixed left-0 top-0 bottom-0 w-64 bg-white border-r border-gray-200 z-50 transition-transform duration-300 ${
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
          <button onClick={() => setSidebarOpen(false)} className="text-gray-400 hover:text-gray-600">
            <span className="sr-only">Close menu</span>
            <div className="w-6 h-6">
              <svg fill="none" stroke="currentColor" viewBox="0 0 24 24" strokeWidth="2">
                <path strokeLinecap="round" strokeLinejoin="round" d="M6 18L18 6M6 6l12 12"></path>
              </svg>
            </div>
          </button>
        </div>

        <nav className="p-4 space-y-1">
          {NAV_ITEMS.filter((item) => item.role === role || item.role === 'CUSTOMER').map((item) => (
            <Link
              key={item.path}
              to={item.path}
              className={`flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium transition-colors ${
                isActive(item.path)
                  ? 'bg-paypal-50 text-paypal-600 border-r-2 border-paypal-600'
                  : 'text-gray-600 hover:bg-gray-50 hover:text-paypal-600'
              }`}
            >
              <item.icon className="w-4 h-4" />
              {item.label}
            </Link>
          ))}
        </nav>

        <div className="mt-auto p-6 border-t border-gray-200">
          <button
            onClick={() => setRole(role === 'CUSTOMER' ? 'MERCHANT' : 'CUSTOMER')}
            className="w-full flex items-center justify-between px-4 py-2.5 rounded-lg border border-gray-200 text-sm font-medium hover:bg-gray-50 transition-colors"
          >
            <span>Switch to {role === 'CUSTOMER' ? 'Merchant' : 'Customer'}</span>
            <ChevronRight className="w-3 h-3" />
          </button>
          <button
            onClick={() => {
              logout();
              window.location.reload();
            }}
            className="w-full flex items-center justify-center px-4 py-2.5 rounded-lg border border-gray-200 text-sm font-medium text-red-600 hover:bg-red-50 mt-2 transition-colors"
          >
            Sign out
          </button>
        </div>
      </aside>

      <header className="sticky top-0 z-40 border-b border-gray-200 bg-white/95 backdrop-blur-md">
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
              <span className="hidden sm:block text-sm text-gray-500">{user.kind}</span>
              <div className="h-8 w-8 rounded-full bg-paypal-600 flex items-center justify-center text-white text-sm font-medium">
                {user.kind?.[0]?.toUpperCase()}
              </div>
            </div>
          </div>
        </div>
      </header>

      <main className={`pt-20 pb-8 px-4 sm:px-6 lg:px-8 ${sidebarOpen ? 'ml-64' : ''}`}>
        {children}
      </main>
    </div>
  );
}
