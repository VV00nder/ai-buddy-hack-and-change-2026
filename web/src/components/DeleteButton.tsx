import { useState } from 'react';

/** Удаление в два шага: «Удалить» → «Удалить? Да / Нет», без системных диалогов. */
export function DeleteButton({ what, pending, onConfirm, label = 'Удалить', note = 'Удалить? Стажёр получит уведомление.' }: { what: string; pending: boolean; onConfirm: () => void; label?: string; note?: string }) {
  const [asking, setAsking] = useState(false);
  if (!asking)
    return (
      <button className="link small req-del" aria-label={`${label} ${what}`} onClick={() => setAsking(true)}>
        {label}
      </button>
    );
  return (
    <span className="req-del confirm" role="group" aria-label={`Подтвердите удаление: ${what}`}>
      <span className="small">{note}</span>
      <button className="btn danger small" disabled={pending} onClick={onConfirm}>Да, {label.toLowerCase()}</button>
      <button className="link small" onClick={() => setAsking(false)}>Нет</button>
    </span>
  );
}

