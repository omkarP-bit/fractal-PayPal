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
    { path: '/demo', label: 'Demo controls', icon: FlaskConical },
  ],
};

function roleFor(user: ReturnType<typeof useAuthStore.getState>['user']): Role {
  return user?.kind === 'MERCHANT_USER' ? 'MERCHANT' : 'CUSTOMER';
}

function Brand({ to }: { to: string }) {
  return (
    <Link to={to} className="flex shrink-0 items-center gap-2.5" aria-label="FRACTAL home">
      <span className="flex h-8 w-8 items-center justify-center rounded-md bg-navy text-[15px] font-semibold text-[color:var(--surface)]">
        F
      </span>
      <span className="hidden text-[15px] font-semibold tracking-[0.1em] text-ink sm:inline">
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

  const isLogin = location.pathname === '/login';
  const role = roleFor(user);
  const homePath = role === 'MERCHANT' ? '/merchant/cases' : '/customer/cases';
  const isDemo = location.pathname.startsWith('/demo');

  useEffect(() => {
    if (!user && !isLogin) navigate('/login', { replace: true });
  }, [user, isLogin, navigate]);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  // "g c" opens the cases list for the current role.
  useEffect(() => {
    if (isLogin) return;
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
  }, [homePath, isLogin, navigate]);

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

  if (isLogin) {
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
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[200] focus:rounded-md focus:bg-surface focus:px-4 focus:py-2 focus:text-body focus:text-blue focus:shadow-pop"
      >
        Skip to content
      </a>

      <header className="sticky top-0 z-40 border-b border-line bg-surface">
        <div className="flex h-16 items-center gap-3 px-4 sm:gap-4 sm:px-6">
          <Brand to={homePath} />

          {isDemo && (
            <span className="hidden items-center gap-1.5 rounded-pill bg-warning-bg px-2.5 py-1 text-[11px] font-medium text-warning sm:inline-flex">
              <FlaskConical className="h-3.5 w-3.5" aria-hidden="true" strokeWidth={1.75} />
              Demo mode
            </span>
          )}

          <div role="search" className="relative mx-auto min-w-0 flex-1 md:max-w-[480px]">
            <Search
              className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted"
              aria-hidden="true"
              strokeWidth={1.75}
            />
            <input
              ref={inputRef}
              type="search"
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="Search cases, customers or orders"
              aria-label="Search cases, customers or orders"
              className="h-10 w-full rounded-pill border border-line bg-surface-muted pl-9 pr-16 text-meta text-ink placeholder:text-muted transition-colors duration-150 focus:border-blue focus:outline-none [&::-webkit-search-cancel-button]:hidden"
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
                className="absolute right-1.5 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-pill text-muted transition-colors hover:bg-surface hover:text-ink"
              >
                <X className="h-3.5 w-3.5" aria-hidden="true" />
              </button>
            ) : (
              <kbd className="pointer-events-none absolute right-2.5 top-1/2 hidden -translate-y-1/2 rounded-sm border border-line bg-surface px-1.5 py-0.5 font-sans text-[11px] text-muted sm:block">
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
                <Sun className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />
              ) : (
                <Moon className="h-4 w-4" aria-hidden="true" strokeWidth={1.75} />
              )}
            </IconButton>

            <div className="relative" ref={menuRef}>
              <button
                type="button"
                onClick={() => setMenuOpen((open) => !open)}
                aria-haspopup="menu"
                aria-expanded={menuOpen}
                className="flex h-10 items-center gap-2 rounded-pill pl-1 pr-2 transition-colors hover:bg-surface-muted"
              >
                <span className="flex h-8 w-8 items-center justify-center rounded-full bg-info-bg text-[12px] font-semibold text-info">
                  {initials(displayName)}
                </span>
                <ChevronDown className="hidden h-3.5 w-3.5 text-muted sm:block" aria-hidden="true" />
                <span className="sr-only">Account menu</span>
              </button>

              {menuOpen && (
                <div
                  role="menu"
                  className="absolute right-0 top-full z-50 mt-2 w-64 animate-slide-up rounded-lg border border-line bg-surface p-1.5 shadow-pop"
                >
                  <div className="border-b border-line px-3 py-3">
                    <p className="truncate text-body font-semibold text-ink">{displayName}</p>
                    {email && <p className="truncate text-meta text-muted">{email}</p>}
                    <span className="mt-2 inline-flex rounded-pill bg-info-bg px-2 py-0.5 text-[11px] font-medium text-info">
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
                    className="mt-1.5 flex w-full items-center gap-2 rounded-sm px-3 py-2 text-left text-body text-ink transition-colors hover:bg-surface-muted"
                  >
                    <Keyboard className="h-4 w-4 text-muted" aria-hidden="true" strokeWidth={1.75} />
                    Keyboard shortcuts
                  </button>
                  <button
                    type="button"
                    role="menuitem"
                    onClick={signOut}
                    className="flex w-full items-center gap-2 rounded-sm px-3 py-2 text-left text-body text-ink transition-colors hover:bg-surface-muted"
                  >
                    <LogOut className="h-4 w-4 text-muted" aria-hidden="true" strokeWidth={1.75} />
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
                    'flex h-10 items-center gap-2.5 rounded-md px-2.5 text-body transition-colors duration-150 lg:px-3',
                    active
                      ? 'bg-info-bg font-medium text-blue'
                      : 'text-muted hover:bg-surface-muted hover:text-ink',
                  )}
                >
                  <item.icon
                    className="h-5 w-5 shrink-0"
                    aria-hidden="true"
                    strokeWidth={1.75}
                  />
                  <span className="hidden truncate lg:inline">{item.label}</span>
                </Link>
              );
            })}
          </nav>
          <p className="hidden px-4 py-4 text-[11px] leading-4 text-muted lg:block">
            Press <kbd className="rounded-sm border border-line px-1">/</kbd> to search
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
        className="fixed inset-x-0 bottom-0 z-40 flex h-16 border-t border-line bg-surface sm:hidden"
      >
        {navItems.map((item) => {
          const active = activePath === item.path;
          return (
            <Link
              key={item.path}
              to={item.path}
              aria-current={active ? 'page' : undefined}
              className={cn(
                'flex flex-1 flex-col items-center justify-center gap-1 text-[11px] transition-colors',
                active ? 'font-semibold text-blue' : 'text-muted',
              )}
            >
              <item.icon className="h-5 w-5" aria-hidden="true" strokeWidth={1.75} />
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
              <dt className="text-body text-ink">{action}</dt>
              <dd>
                <kbd className="rounded-sm border border-line bg-surface-muted px-1.5 py-0.5 font-sans text-[11px] text-muted">
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
