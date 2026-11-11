import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Mail, Users, ArrowRight } from '../components/ui/Icons';
import { useAuthStore } from '../store/auth';
import { Button } from '../components/ui/Button';
import { Input } from '../components/ui/Input';

interface RoleOption {
  value: 'CUSTOMER' | 'MERCHANT';
  label: string;
  icon: React.ComponentType<{ className?: string }>;
}

const ROLES: RoleOption[] = [
  {
    value: 'CUSTOMER',
    label: 'Customer',
    icon: Mail,
    description: 'Report a payment issue as a buyer',
  },
  {
    value: 'MERCHANT',
    label: 'Merchant',
    icon: Users,
    description: 'Manage cases for your store',
  },
];

export function LoginPage() {
  const [role, setRole] = useState<'CUSTOMER' | 'MERCHANT'>('CUSTOMER');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const navigate = useNavigate();
  const login = useAuthStore((s) => s.login);

  const handleLogin = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email) {
      setError('Please enter your email');
      return;
    }
    setLoading(true);
    setError('');

    try {
      const token = `dev-token-${role}-${email}-${Date.now()}`;
      const user = {
        sub: email,
        kind: role === 'CUSTOMER' ? 'CUSTOMER' : 'MERCHANT_USER',
        merchant_id: role === 'MERCHANT' ? 'mer-1' : undefined,
        role: role === 'MERCHANT' ? 'MERCHANT_ADMIN' : undefined,
        permissions: role === 'MERCHANT' ? ['reconcile', 'refund'] : [],
      };

      login(token, user);

      if (role === 'CUSTOMER') {
        navigate('/customer/cases');
      } else {
        navigate('/merchant/cases');
      }
    } catch (err: any) {
      setError(err.message || 'Login failed');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-gray-50">
      <div className="w-full max-w-lg space-y-8">
        <div className="flex items-center justify-center">
          <div className="bg-paypal-600 rounded-xl p-3 flex items-center justify-center shadow-lg shadow-paypal-600/20">
            <span className="text-white font-bold text-2xl tracking-tight">F</span>
          </div>
        </div>
        <h2 className="text-2xl font-bold text-gray-900 text-center">Sign in to FRACTAL</h2>
        <p className="text-gray-600 text-center">
          Select your role to continue
        </p>

        <div className="grid grid-cols-2 gap-3 mb-6">
          {ROLES.map((roleOpt) => (
            <button
              key={roleOpt.value}
              onClick={() => setRole(roleOpt.value)}
              className={`flex flex-col items-center gap-2 px-4 py-4 rounded-xl text-sm font-medium transition-all border ${
                role === roleOpt.value
                  ? 'bg-paypal-50 border-2 border-paypal-600 text-paypal-600 shadow-sm dark:text-paypal-300'
                  : 'bg-white dark:bg-gray-800 border border-gray-200 text-gray-600 hover:border-paypal-600'
              }`}
            >
              <roleOpt.icon className="w-5 h-5" />
              <span>{roleOpt.label}</span>
              <span className="text-xs">{roleOpt.description}</span>
            </button>
          ))}
        </div>

        {error && (
          <div className="bg-red-50 border border-red-200 rounded-lg p-3 text-sm text-red-700">
            {error}
          </div>
        )}

        <form onSubmit={handleLogin} className="space-y-4">
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">
              Email address
            </label>
            <input
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              className="input-field"
              disabled={loading}
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-700 mb-1.5">
              Password
            </label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              className="input-field"
              disabled={loading}
            />
          </div>
          <button
            type="submit"
            disabled={loading}
            className="btn-primary w-full flex items-center justify-center gap-2 py-3"
          >
            {loading ? (
              <span className="flex items-center gap-2">
                <svg className="w-4 h-4 animate-spin" viewBox="0 0 24 24">
                  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
                  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938V12a6 6 0 016-6 6 6 0 016 6 6 6 0 016 6 6 6 0 016 6 6 6 0 016 6 6 6v4c0-2.007-1.179-3.93-3-5.938V12c0-1.849-.729-3.473-1.85-4.754z"></path>
                </svg>
                Signing in...
              </span>
            ) : (
              <span className="flex items-center gap-2">
                <ArrowRight className="w-4 h-4" /> Continue
              </span>
            )}
          </button>
        </form>

        <p className="text-xs text-gray-500 text-center">
          Dev mode: No password required. Select a role and sign in.
        </p>

        <div className="pt-4 border-t border-gray-200">
          <p className="text-xs text-gray-500 text-center">Demo Credentials</p>
          <div className="grid grid-cols-2 gap-2 mt-2">
            <div className="text-xs bg-gray-100 p-2 rounded border border-gray-200">
              <span className="text-blue-600 font-bold text-xs">Customer:</span><br />
              buyer@example.com / password
            </div>
            <div className="text-xs bg-gray-100 p-2 rounded border border-gray-200">
              <span className="text-blue-600 font-bold text-xs">Merchant:</span><br />
              ops@acme.com / password
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
