import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
  type RefObject,
} from 'react';

interface GlobalSearchValue {
  query: string;
  setQuery: (value: string) => void;
  clear: () => void;
  inputRef: RefObject<HTMLInputElement>;
}

const GlobalSearchContext = createContext<GlobalSearchValue | null>(null);

export function GlobalSearchProvider({ children }: { children: ReactNode }) {
  const [query, setQuery] = useState('');
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const typing =
        target?.tagName === 'INPUT' ||
        target?.tagName === 'TEXTAREA' ||
        target?.tagName === 'SELECT' ||
        target?.isContentEditable;

      if (event.key === '/' && !typing && !event.metaKey && !event.ctrlKey) {
        event.preventDefault();
        inputRef.current?.focus();
        inputRef.current?.select();
      }

      if (event.key === 'Escape') {
        const focused = document.activeElement === inputRef.current;
        setQuery('');
        if (focused) inputRef.current?.blur();
      }
    };

    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, []);

  const value = useMemo<GlobalSearchValue>(
    () => ({
      query,
      setQuery,
      clear: () => setQuery(''),
      inputRef,
    }),
    [query],
  );

  return <GlobalSearchContext.Provider value={value}>{children}</GlobalSearchContext.Provider>;
}

export function useGlobalSearch(): GlobalSearchValue {
  const context = useContext(GlobalSearchContext);
  if (!context) {
    throw new Error('useGlobalSearch must be used inside GlobalSearchProvider');
  }
  return context;
}

/**
 * Highlight every case-insensitive occurrence of `query` in `text`.
 * Returns plain strings and <mark> elements so callers can render in any cell.
 */
export function splitHighlight(text: string, query: string): Array<{ match: boolean; text: string }> {
  const needle = query.trim();
  if (!needle) return [{ match: false, text }];
  const lowerText = text.toLowerCase();
  const lowerNeedle = needle.toLowerCase();
  const parts: Array<{ match: boolean; text: string }> = [];
  let index = 0;

  while (index < text.length) {
    const found = lowerText.indexOf(lowerNeedle, index);
    if (found === -1) {
      parts.push({ match: false, text: text.slice(index) });
      break;
    }
    if (found > index) parts.push({ match: false, text: text.slice(index, found) });
    parts.push({ match: true, text: text.slice(found, found + needle.length) });
    index = found + needle.length;
  }

  return parts;
}
