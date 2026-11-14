// Older builds saved maxWidth through a text input, so stored values may be
// numbers or numeric strings. Empty and invalid values stay unset; zero is kept.

export function normalizeMaxWidth(value: unknown): number | undefined {
  if (typeof value === 'number')
    return Number.isFinite(value) ? value : undefined
  if (typeof value !== 'string')
    return undefined
  const trimmed = value.trim()
  if (trimmed === '')
    return undefined
  const parsed = Number(trimmed)
  return Number.isFinite(parsed) ? parsed : undefined
}

export function maxWidthControlValue(value: unknown): number | null {
  const width = normalizeMaxWidth(value)
  return width === undefined ? null : width
}

export function maxWidthFromControl(value: number | null): number | undefined {
  return normalizeMaxWidth(value)
}

export function contentMaxWidth(value: unknown, unit: string): string {
  const width = normalizeMaxWidth(value) ?? 1200
  return `${width}${unit}`
}
