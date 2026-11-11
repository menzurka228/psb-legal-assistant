import mascot from "./assets/iskra.png";

const features = [
  { icon: "document", title: "Договоры и регламенты", text: "Найдите нужный пункт во внутренних документах и уточните порядок согласования." },
  { icon: "clock", title: "Редакции на нужную дату", text: "Укажите дату события, чтобы «Искра» учла, какие нормы тогда действовали." },
  { icon: "link", title: "Ссылки на источники", text: "Откройте положение из ответа и проверьте его в полном тексте документа." },
] as const;

function Icon({ type }: { type: typeof features[number]["icon"] }) {
  return (
    <svg aria-hidden="true" width="23" height="23" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round">
      {type === "document" && <path d="M13 3H6a1 1 0 0 0-1 1v16a1 1 0 0 0 1 1h12a1 1 0 0 0 1-1V9m-6-6 6 6h-6V3ZM8.5 13h7m-7 4h5" />}
      {type === "clock" && <><circle cx="12" cy="12" r="8.5" /><path d="M12 7.5V12l3 2" /></>}
      {type === "link" && <path d="M10 14l4-4m-6.5 2.5-2 2a3 3 0 0 0 4.25 4.25l3-3m-1.5-7.5 3-3a3 3 0 0 1 4.25 4.25l-2 2" />}
    </svg>
  );
}

export function Landing() {
  return (
    <>
      <div className="landing" id="top">
        <header className="site-header frame">
          <a className="brand" href="#top"><span aria-hidden="true" className="brand-mark">✦</span> Искра</a>
          <nav aria-label="Основная навигация">
            <a className="nav-about" href="#capabilities">Возможности</a>
            <a className="nav-chat" href="#assistant">Открыть чат <span aria-hidden="true">↗</span></a>
          </nav>
        </header>
        <section className="hero frame" aria-labelledby="hero-title">
          <div className="hero-copy">
            <p className="eyebrow">ИИ-АССИСТЕНТ ДЛЯ ЮРИСТОВ ПСБ</p>
            <h1 id="hero-title">Задайте вопрос<br /> <span>«Искре».</span></h1>
            <p className="hero-description">Находит нужные положения в договорах, регламентах и законах. Показывает, на чём основан ответ.</p>
            <a className="primary-cta" href="#assistant">Спросить «Искру» <span aria-hidden="true">↗</span></a>
          </div>
          <div className="hero-art">
            <div className="orbit orbit-one" />
            <div className="orbit orbit-two" />
            <span className="glint glint-one" aria-hidden="true">✦</span>
            <span className="glint glint-two" aria-hidden="true">✦</span>
            <img src={mascot} alt="Улыбающийся маскот Искры" fetchPriority="high" />
          </div>
        </section>
      </div>
      <section className="features frame" id="capabilities" aria-label="Возможности Искры">
        {features.map((feature) => (
          <article key={feature.icon} className="feature">
            <div className="feature-icon"><Icon type={feature.icon} /></div>
            <h2>{feature.title}</h2>
            <p>{feature.text}</p>
          </article>
        ))}
      </section>
    </>
  );
}
