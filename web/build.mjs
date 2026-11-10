// Сборка фронтенда: esbuild → dist/ (статические файлы раздаёт Go-сервис).
import * as esbuild from 'esbuild';
import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';

const watch = process.argv.includes('--watch');
rmSync('dist', { recursive: true, force: true });
mkdirSync('dist/assets', { recursive: true });
cpSync('public', 'dist', { recursive: true });

const options = {
  entryPoints: ['src/main.tsx'],
  bundle: true,
  outdir: 'dist/assets',
  entryNames: '[name]-[hash]',
  format: 'esm',
  target: ['es2022'],
  minify: !watch,
  sourcemap: true,
  metafile: true,
  jsx: 'automatic',
  define: { 'process.env.NODE_ENV': JSON.stringify(watch ? 'development' : 'production') },
  logLevel: 'info',
};

function writeIndex(meta) {
  const outputs = Object.keys(meta.outputs);
  const js = outputs.find((f) => f.endsWith('.js')).replace('dist/', '/');
  const css = outputs.find((f) => f.endsWith('.css'))?.replace('dist/', '/');
  const html = readFileSync('public/index.html', 'utf8')
    .replace('<!--CSS-->', css ? `<link rel="stylesheet" href="${css}">` : '')
    .replace('<!--JS-->', `<script type="module" src="${js}"></script>`);
  writeFileSync('dist/index.html', html);
}

if (watch) {
  const ctx = await esbuild.context({
    ...options,
    plugins: [{ name: 'index', setup: (b) => b.onEnd((r) => r.metafile && writeIndex(r.metafile)) }],
  });
  await ctx.watch();
} else {
  const result = await esbuild.build(options);
  writeIndex(result.metafile);
}
