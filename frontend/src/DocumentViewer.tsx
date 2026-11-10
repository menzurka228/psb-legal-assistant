import { useEffect, useRef, useState } from "react";
import { api, type Source } from "./api";

// Полный текст документа с подсвеченным фрагментом-источником
export function DocumentViewer({ source, onClose }: { source: Source; onClose: () => void }) {
  const [text, setText] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const markRef = useRef<HTMLElement>(null);

  useEffect(() => {
    api.document(source.doc).then((d) => setText(d.text)).catch((e) => setError(e.message));
  }, [source.doc]);

  useEffect(() => {
    markRef.current?.scrollIntoView({ block: "center" });
  }, [text]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  const start = text ? text.indexOf(source.text.slice(0, 200)) : -1;

  return (
    <div className="modal-bg" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header>
          <h3>{source.docTitle}</h3>
          <button className="ghost" onClick={onClose}>Закрыть</button>
        </header>
        <div className="doc-body">
          {error && <p className="error">{error}</p>}
          {text === null && !error && <p className="muted">Загрузка…</p>}
          {text !== null &&
            (start < 0 ? (
              text
            ) : (
              <>
                {text.slice(0, start)}
                <mark ref={markRef}>{text.slice(start, start + source.text.length)}</mark>
                {text.slice(start + source.text.length)}
              </>
            ))}
        </div>
      </div>
    </div>
  );
}
