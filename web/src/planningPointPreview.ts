export function shortMarkdownPreview(markdown: string | undefined, maxChars = 72): string {
  if (!markdown?.trim() || maxChars < 1) return ''
  const text = markdown
    .replace(/!\[[^\]]*\]\([^)]*\)/g, ' ')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/<[^>]*>/g, ' ')
    .replace(/^\s{0,3}#{1,6}\s+/gm, '')
    .replace(/^\s*[-*+]\s+/gm, '')
    .replace(/[`*_~>#]/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
  const chars = [...text]
  return chars.length > maxChars ? chars.slice(0, maxChars - 1).join('') + '…' : text
}
