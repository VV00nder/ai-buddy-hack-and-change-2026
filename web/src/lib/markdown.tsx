// Безопасный мини-рендерер ответов ассистента: абзацы, списки, **жирный**,
// _курсив_ и `код`. HTML из текста никогда не вставляется как разметка.
import type { ReactNode } from 'react';

export type Block = { type: 'p'; lines: string[] } | { type: 'ul'; items: string[] } | { type: 'ol'; items: string[] };

export function parseBlocks(text: string): Block[] {
  const blocks: Block[] = [];
  for (const raw of text.replace(/\r\n/g, '\n').split('\n')) {
    const line = raw.trimEnd();
    const last = blocks[blocks.length - 1];
    const bullet = /^\s*[-*•]\s+(.*)$/.exec(line);
    const numbered = /^\s*\d+[.)]\s+(.*)$/.exec(line);
    if (bullet) {
      if (last?.type === 'ul') last.items.push(bullet[1]!);
      else blocks.push({ type: 'ul', items: [bullet[1]!] });
    } else if (numbered) {
      if (last?.type === 'ol') last.items.push(numbered[1]!);
      else blocks.push({ type: 'ol', items: [numbered[1]!] });
    } else if (line.trim() === '') {
      blocks.push({ type: 'p', lines: [] });
    } else if (last?.type === 'p') {
      last.lines.push(line);
    } else {
      blocks.push({ type: 'p', lines: [line] });
    }
  }
  return blocks.filter((b) => b.type !== 'p' || b.lines.length > 0);
}

const inlineRe = /(\*\*[^*]+\*\*|`[^`]+`|_[^_]+_)/g;

export function inline(text: string): ReactNode[] {
  return text.split(inlineRe).map((part, i) => {
    if (part.startsWith('**') && part.endsWith('**') && part.length > 4) return <strong key={i}>{part.slice(2, -2)}</strong>;
    if (part.startsWith('`') && part.endsWith('`') && part.length > 2) return <code key={i}>{part.slice(1, -1)}</code>;
    if (part.startsWith('_') && part.endsWith('_') && part.length > 2) return <em key={i}>{part.slice(1, -1)}</em>;
    return part;
  });
}

export function Markdown({ text }: { text: string }) {
  return (
    <>
      {parseBlocks(text).map((b, i) => {
        if (b.type === 'ul') return <ul key={i}>{b.items.map((it, j) => <li key={j}>{inline(it)}</li>)}</ul>;
        if (b.type === 'ol') return <ol key={i}>{b.items.map((it, j) => <li key={j}>{inline(it)}</li>)}</ol>;
        return (
          <p key={i}>
            {b.lines.map((l, j) => (
              <span key={j}>
                {j > 0 && <br />}
                {inline(l)}
              </span>
            ))}
          </p>
        );
      })}
    </>
  );
}
