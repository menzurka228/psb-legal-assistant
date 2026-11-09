export type Role = "user" | "assistant";

export interface Message {
  role: Role;
  content: string;
}

export interface Source {
  n: number;
  id: string;
  doc: string;
  title: string;
  text: string;
  score: number;
}

export interface ChatResponse {
  answer: string;
  sources: Source[];
  refused: boolean;
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error ?? `Ошибка ${res.status}`);
  return data as T;
}

export const api = {
  chat: (question: string, history: Message[]) =>
    request<ChatResponse>("/api/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ question, history }),
    }),
  document: (name: string) =>
    request<{ name: string; text: string }>(`/api/documents/${encodeURIComponent(name)}`),
};
