import { splitHighlight } from '../../lib/search';

/** Renders text with case-insensitive search matches wrapped in <mark>. */
export function Highlight({ text, query }: { text: string; query: string }) {
  return (
    <>
      {splitHighlight(text, query).map((part, index) =>
        part.match ? (
          <mark key={index} className="rounded-sm border border-line bg-yellow px-0.5 font-bold text-[color:var(--text-on-yellow)]">
            {part.text}
          </mark>
        ) : (
          <span key={index}>{part.text}</span>
        ),
      )}
    </>
  );
}
