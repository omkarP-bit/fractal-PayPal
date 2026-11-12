import { useEffect, useRef, useState } from 'react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  ChevronDown,
  FilePlus2,
  FlaskConical,
  Inbox,
  Keyboard,
  LogOut,
  Moon,
  PlugZap,
  Search,
  Sun,
  X,
} from 'lucide-react';
import { cn } from '../lib/cn';
import { applyTheme, resolveTheme, type Theme } from '../lib/theme';
import { useGlobalSearch, GlobalSearchProvider } from '../lib/search';
import { useAuthStore } from '../store/auth';
import { IconButton } from './ui/Button';
import { Dialog } from './ui/Dialog';
import { initials } from '../lib/format';

type Role = 'CUSTOMER' | 'MERCHANT';

interface NavItem {
  path: string;
  label: string;
  icon: typeof Inbox;
}

const NAV_ITEMS: Record<Role, NavItem[]> = {
  CUSTOMER: [
    { path: '/customer/cases', label: 'My cases', icon: Inbox },
    { path: '/customer/cases/new', label: 'Report an issue', icon: FilePlus2 },
  ],
  MERCHANT: [
    { path: '/merchant/cases', label: 'Cases', icon: Inbox },
    { path: '/merchant/onboarding', label: 'PayPal setup', icon: PlugZap },
    { path: '/demo', label: 'Demo controls', icon: FlaskConical },
  ],
};

function roleFor(user: ReturnType<typeof useAuthStore.getState>['user']): Role {
  return user?.kind === 'MERCHANT_USER' ? 'MERCHANT' : 'CUSTOMER';
}

function Brand({ to }: { to: string }) {
  return (
    <Link to={to} className="flex shrink-0 items-center gap-2.5" aria-label="FRACTAL home">
      <span className="flex h-9 w-9 items-center justify-center rounded border-2 border-line bg-[#003087] text-[16px] font-extrabold text-white shadow-hard">
        F
      </span>
      <span className="hidden text-[15px] font-extrabold uppercase tracking-[0.12em] text-ink sm:inline">
        FRACTAL
      </span>
    </Link>
  );
}

function Shell() {
  const location = useLocation();
  const navigate = useNavigate();
  const { user, logout } = useAuthStore();
  const { query, setQuery, clear, inputRef } = useGlobalSearch();
  const [theme, setTheme] = useState<Theme>(() => resolveTheme());
  const [menuOpen, setMenuOpen] = useState(false);
  const [shortcutsOpen, setShortcutsOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  const isPublic = location.pathname === '/login' || location.pathname === '/signup';
  const role = roleFor(user);
  const homePath = role === 'MERCHANT' ? '/merchant/cases' : '/customer/cases';
  const isDemo = location.pathname.startsWith('/demo');

  useEffect(() => {
    if (!user && !isPublic) navigate('/login', { replace: true });
  }, [user, isPublic, navigate]);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  // "g c" opens the cases list for the current role.
  useEffect(() => {
    if (isPublic) return;
    let lastG = 0;
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const typing =
        target?.tagName === 'INPUT' ||
        target?.tagName === 'TEXTAREA' ||
        target?.tagName === 'SELECT' ||
        target?.isContentEditable;
      if (typing || event.metaKey || event.ctrlKey || event.altKey) return;

      if (event.key.toLowerCase() === 'g') {
        lastG = Date.now();
        return;
      }
      if (event.key.toLowerCase() === 'c' && Date.now() - lastG < 1200) {
        lastG = 0;
        navigate(homePath);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [homePath, isPublic, navigate]);

  useEffect(() => {
    if (!menuOpen) return;
    const onPointerDown = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setMenuOpen(false);
      }
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false);
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [menuOpen]);

  if (isPublic) {
    return (
      <main id="main-content" className="min-h-screen bg-bg">
        <Outlet />
      </main>
    );
  }

  if (!user) return null;

  const navItems = NAV_ITEMS[role];
  const activePath = navItems
    .map((item) => item.path)
    .filter(
      (path) => location.pathname === path || location.pathname.startsWith(`${path}/`),
    )
    .sort((a, b) => b.length - a.length)[0];

  const displayName = user.display_name || user.email || 'Account';
  const email = user.email ?? '';
  const roleLabel = role === 'MERCHANT' ? 'Merchant admin' : 'Customer';
  const signOut = () => {
    logout();
    navigate('/login');
  };

  return (
    <div className="app-frame bg-bg">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[200] focus:rounded-md focus:bg-surface focus:px-4 focus:py-2 focus:text-body focus:text-blue focus:shadow-hard"
      >
        Skip to content
      </a>

      <header className="sticky top-0 z-40 border-b-2 border-line bg-surface">
        <div className="flex h-16 items-center gap-3 px-4 sm:gap-4 sm:px-6">
          <Brand to={homePath} />

          {isDemo && (
            <span className="hidden items-center gap-1.5 rounded-pill border-2 border-line bg-warning px-2 py-0.5 text-[11px] font-bold uppercase tracking-[0.04em] text-[color:var(--warning-ink)] shadow-hard sm:inline-flex">
              <FlaskConical className="h-3.5 w-3.5" aria-hidden="true" strokeWidth={2.5} />
              Demo mode
            </span>
          )}

          <div role="search" className="relative mx-auto min-w-0 flex-1 md:max-w-[560px]">
            <Search
              className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted"
              aria-hidden="true"
              strokeWidth={2}
            />
            <input
              ref={inputRef}
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search cases, customers or orders"
              aria-label="Search cases, customers or orders"
              className="h-12 w-full rounded border-2 border-line bg-surface pl-10 pr-16 text-body font-medium text-ink placeholder:text-muted/70 transition-shadow duration-100 focus:shadow-[4px_4px_0_var(--brand-sky)] focus:outline-none [&::-webkit-search-cancel-button]:hidden"
            />
            {query ? (
              <button
                type="button"
                onClick={() => {
                  clear();
                  inputRef.current?.focus();
                }}
                aria-label="Clear search"
                title="Clear search"
                className="absolute right-1.5 top-1/2 flex h-9 w-9 -translate-y-1/2 items-center justify-center rounded text-ink transition-colors hover:bg-surface-alt"
              >
                <X className="h-4 w-4" aria-hidden="true" />
              </button>
            ) : (
              <kbd className="pointer-events-none absolute right-2.5 top-1/2 hidden -translate-y-1/2 rounded-pill border border-line bg-surface-alt px-1.5 py-0.5 font-mono text-[11px] font-bold text-ink sm:block">
                /
              </kbd>
            )}
          </div>

          <div className="flex shrink-0 items-center gap-1">
            <IconButton
              label={theme === 'dark' ? 'Switch to light mode' : 'Switch to dark mode'}
              role="switch"
              aria-checked={theme === 'dark'}
              onClick={() => {
                const next = theme === 'dark' ? 'light' : 'dark';
                applyTheme(next, true);
                setTheme(next);
              }}
            >
              {theme === 'dark' ? (
                <Sun className="h-5 w-5" aria-hidden="true" strokeWidth={2} />
              ) : (
                <Moon className="h-5 w-5" aria-hidden="true" strokeWidth={2} />
              )}
            </IconButton>

            <div className="relative" ref={menuRef}>
              <button
                type="button"
                onClick={() => setMenuOpen((open) => !open)}
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                className="flex h-11 items-center gap-2 rounded px-1.5 transition-colors hover:bg-surface-alt"
              >
                <span className="flex h-9 w-9 items-center justify-center rounded border-2 border-line bg-info text-[12px] font-extrabold text-[color:var(--info-ink)]">
                  {initials(displayName)}
                </span>
                <ChevronDown className="hidden h-4 w-4 text-ink sm:block" aria-hidden="true" strokeWidth={2.5} />
                <span className="sr-only">Account menu</span>
              </button>

              {menuOpen && (
                <div
                  role="menu"
                  className="absolute right-0 top-full z-50 mt-2 w-72 animate-slide-up rounded border-2 border-line bg-surface p-1.5 shadow-hard-lg"
                >
                  <div className="border-b-2 border-line px-3 py-3">
                    <p className="truncate text-body font-bold text-ink">{displayName}</p>
                    {email && <p className="truncate font-mono text-[12px] text-muted">{email}</p>}
                    <span className="mt-2 inline-flex rounded-pill border-2 border-line bg-blue px-2 py-0.5 text-[11px] font-bold uppercase tracking-[0.04em] text-white">
                      {roleLabel}
                    </span>
                  </div>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={() => {
                      setMenuOpen(false);
                      setShortcutsOpen(true);
                    }}
                    className="mt-1.5 flex w-full items-center gap-2 rounded px-3 py-2 text-left text-body font-medium text-ink transition-colors hover:bg-surface-alt"
                  >
                    <Keyboard className="h-4 w-4 text-ink" aria-hidden="true" strokeWidth={2} />
                    Keyboard shortcuts
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={signOut}
                    className="flex w-full items-center gap-2 rounded px-3 py-2 text-left text-body font-medium text-ink transition-colors hover:bg-surface-alt"
                  >
                    <LogOut className="h-4 w-4 text-ink" aria-hidden="true" strokeWidth={2} />
                    Sign out
                  </button>
                </div>
              )}
            </div>
          </div>
        </div>
      </header>

      <div className="app-shell">
        <aside className="sticky top-16 hidden h-[calc(100dvh-4rem)] min-w-0 flex-col border-r border-line bg-surface sm:flex">
          <nav aria-label="Main" className="flex flex-1 flex-col gap-1 p-2 lg:p-3">
            {navItems.map((item) => {
              const active = activePath === item.path;
              return (
                <Link
                  key={item.path}
                  to={item.path}
                  title={item.label}
                  aria-current={active ? 'page' : undefined}
                  className={cn(
                    'flex h-12 items-center gap-3 rounded border-2 px-2.5 text-body transition-[background-color,box-shadow,transform] duration-100 lg:px-3',
                    active
                      ? 'border-line bg-yellow font-extrabold text-[color:var(--text-on-yellow)] shadow-hard'
                      : 'border-transparent text-ink hover:bg-surface-alt',
                  )}
                >
                  <item.icon
                    className="h-5 w-5 shrink-0"
                    aria-hidden="true"
                    strokeWidth={2}
                  />
                  <span className="hidden truncate lg:inline">{item.label}</span>
                </Link>
              );
            })}
          </nav>
          <p className="hidden px-4 py-4 text-[11px] leading-5 text-muted lg:block">
            Press{' '}
            <kbd className="rounded-pill border border-line bg-surface-alt px-1.5 py-0.5 font-mono font-bold text-ink">
              /
            </kbd>{' '}
            to search
          </p>
        </aside>

        <main
          id="main-content"
          className="app-main"
        >
          <Outlet />
        </main>
      </div>

      <nav
        aria-label="Main"
        className="fixed inset-x-0 bottom-0 z-40 flex h-16 border-t-2 border-line bg-surface sm:hidden"
      >
        {navItems.map((item) => {
          const active = activePath === item.path;
          return (
            <Link
              key={item.path}
              to={item.path}
              aria-current={active ? 'page' : undefined}
              className={cn(
                'flex min-h-[44px] flex-1 flex-col items-center justify-center gap-1 text-[11px] font-bold transition-colors',
                active ? 'bg-yellow text-[color:var(--text-on-yellow)]' : 'text-ink',
              )}
            >
              <item.icon className="h-5 w-5" aria-hidden="true" strokeWidth={2} />
              {item.label}
            </Link>
          );
        })}
      </nav>

      <Dialog
        open={shortcutsOpen}
        onClose={() => setShortcutsOpen(false)}
        title="Keyboard shortcuts"
        description="Work faster without leaving the keyboard."
        size="sm"
      >
        <dl className="divide-y divide-line">
          {[
            ['/', 'Focus search'],
            ['g then c', 'Go to cases'],
            ['Esc', 'Close a dialog or clear search'],
            ['Enter', 'Send a message'],
            ['Shift + Enter', 'Add a new line'],
          ].map(([keys, action]) => (
            <div key={keys} className="flex items-center justify-between gap-4 py-2.5">
              <dt className="text-body font-medium text-ink">{action}</dt>
              <dd>
                <kbd className="rounded-pill border border-line bg-surface-alt px-1.5 py-0.5 font-mono text-[11px] font-bold text-ink">
                  {keys}
                </kbd>
              </dd>
            </div>
          ))}
        </dl>
      </Dialog>
    </div>
  );
}

export function Layout() {
  return (
    <GlobalSearchProvider>
      <Shell />
    </GlobalSearchProvider>
  );
}
