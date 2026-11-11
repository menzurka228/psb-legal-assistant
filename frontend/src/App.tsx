import { useEffect, useRef, useState } from "react";
import { api, type ChatResponse, type Message, type Source } from "./api";
import { Landing } from "./Landing";
import { Answer } from "./Answer";
import { DocumentViewer } from "./DocumentViewer";

interface Turn {
  question: string;
  response?: ChatResponse;
  error?: string;
}

const EXAMPLES = [
  "Какой гарантийный срок по договору с «Альфа-Трейд»?",
  "Договор на 12 млн рублей заключали в августе 2024 года. Нужно ли было одобрение генерального директора?",
  "Какой МРОТ был в 2024 году?",
  "Как согласовать договор поставки с иностранной компанией на 5 млн рублей?",
];

export default function App() {
  const [turns, setTurns] = useState<Turn[]>([]);
  const [input, setInput] = useState("");
  const [loading, setLoading] = useState(false);
  const [active, setActive] = useState<number | null>(null); // индекс хода, чьи источники показаны
  const [highlight, setHighlight] = useState<number | null>(null);
  const [openDoc, setOpenDoc] = useState<Source | null>(null);
  const logRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    logRef.current?.scrollTo({ top: logRef.current.scrollHeight, behavior: "smooth" });
  }, [turns]);

  const history: Message[] = turns.flatMap((t) =>
    t.response
      ? [
          { role: "user" as const, content: t.question },
          { role: "assistant" as const, content: t.response.answer },
        ]
      : [],
  );

  async function ask(question: string) {
    question = question.trim();
    if (!question || loading) return;
    const idx = turns.length;
    setTurns((t) => [...t, { question }]);
    setInput("");
    setLoading(true);
    try {
      const response = await api.chat(question, history);
      setTurns((t) => t.map((x, i) => (i === idx ? { ...x, response } : x)));
      setActive(idx);
      setHighlight(null);
    } catch (e) {
      setTurns((t) => t.map((x, i) => (i === idx ? { ...x, error: (e as Error).message } : x)));
    } finally {
      setLoading(false);
    }
  }

  // На мобильном панель источников под чатом — прокручиваем к выбранному положению
  useEffect(() => {
    if (highlight !== null) document.getElementById(`src-${highlight}`)?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [highlight, active]);

  const sources = active !== null ? (turns[active]?.response?.sources ?? []) : [];

  return (
    <>
    <main>
      <Landing />
      <section className="layout frame" id="assistant" aria-labelledby="chat-title">
        <div className="chat">
          <header>
            <div>
              <h2 id="chat-title">Чат с «Искрой»</h2>
              <p>Ответы только по базе нормативных документов — со ссылками на положения</p>
            </div>
            {turns.length > 0 && (
              <button className="ghost" disabled={loading} onClick={() => { setTurns([]); setActive(null); setHighlight(null); }}>
                Новый диалог
              </button>
            )}
          </header>

          <div className="log" ref={logRef} role="log" aria-label="История диалога">
            {turns.length === 0 && (
              <div className="empty">
                <div className="chat-greeting">
                  <span className="iskra-avatar" aria-hidden="true" />
                  <p>С чего начнём?</p>
                </div>
                {EXAMPLES.map((q) => (
                  <button key={q} className="example" onClick={() => ask(q)}>{q}</button>
                ))}
              </div>
            )}
            {turns.map((t, i) => (
              <div key={i} className="turn">
                <div className="msg user">{t.question}</div>
                <div className="iskra-reply" aria-label="Искра">
                  <span className="iskra-avatar" aria-hidden="true" />
                  <div className="reply-content">
                    {t.response && (
                      <div
                        className={`msg bot ${t.response.refused ? "refused" : ""} ${active === i ? "active" : ""}`}
                        onClick={() => setActive(i)}
                      >
                        {t.response.refused && <span className="badge">Без ссылок на базу</span>}
                        <Answer
                          text={t.response.answer}
                          onCite={(n) => { setActive(i); setHighlight(n); }}
                        />
                      </div>
                    )}
                    {t.error && <div className="msg error">Ошибка: {t.error}</div>}
                    {!t.response && !t.error && <div className="msg bot pending">Ищу в базе знаний…</div>}
                  </div>
                </div>
              </div>
            ))}
          </div>

          <form className="composer" onSubmit={(e) => { e.preventDefault(); ask(input); }}>
            <textarea
              aria-label="Ваш вопрос"
              value={input}
              placeholder="Задайте вопрос…"
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) { e.preventDefault(); ask(input); }
              }}
            />
            <button disabled={loading || !input.trim()}>{loading ? "…" : "Спросить"}</button>
          </form>
        </div>

        <aside className="sources">
          <h2>Подтверждающие положения</h2>
          {sources.length === 0 ? (
            <p className="muted">Здесь появятся фрагменты документов, на которые опирается ответ.</p>
          ) : (
            sources.map((s) => (
              <article key={s.id} id={`src-${s.n}`} className={`source ${highlight === s.n ? "hl" : ""}`}>
                <div className="source-head">
                  <span className="num">[{s.n}]</span>
                  <span className="doc">{s.docTitle}</span>
                </div>
                <div className="title">{s.title}</div>
                <p>{s.text}</p>
                <button className="link" onClick={() => setOpenDoc(s)}>Открыть документ →</button>
              </article>
            ))
          )}
        </aside>

        {openDoc && <DocumentViewer source={openDoc} onClose={() => setOpenDoc(null)} />}
      </section>
    </main>
    <footer className="site-footer frame"><a className="brand" href="#top"><span aria-hidden="true" className="brand-mark">✦</span> Искра</a><span>Юридический ассистент ПСБ</span></footer>
    </>
  );
}
