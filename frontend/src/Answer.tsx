import ReactMarkdown from "react-markdown";

// Ссылки вида [1] превращаем в кликабельные метки, ведущие к источнику
export function Answer({ text, onCite }: { text: string; onCite: (n: number) => void }) {
  const withLinks = text.replace(/\[(\d+)\](?!\()/g, "[[$1]](#cite-$1)");
  return (
    <div className="answer">
      <ReactMarkdown
        components={{
          a: ({ href, children }) => {
            const m = href?.match(/^#cite-(\d+)$/);
            if (!m) return <a href={href} target="_blank" rel="noreferrer">{children}</a>;
            return (
              <button className="cite" onClick={(e) => { e.stopPropagation(); onCite(Number(m[1])); }}>
                {children}
              </button>
            );
          },
        }}
      >
        {withLinks}
      </ReactMarkdown>
    </div>
  );
}
