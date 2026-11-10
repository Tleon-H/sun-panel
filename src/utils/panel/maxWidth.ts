// maxWidth is stored as a number. The settings control can also be cleared.
// An empty control is not a string and is not zero: the page then uses 1200.

export function maxWidthControlValue(value: number | undefined): number | null {
  if (typeof value !== 'number' || Number.isNaN(value))
    return null
  return value
}

export function maxWidthFromControl(value: number | null): number | undefined {
  if (typeof value !== 'number' || Number.isNaN(value))
    return undefined
  return value
}

export function contentMaxWidth(value: number | undefined, unit: string): string {
  const width = maxWidthControlValue(value) ?? 1200
  return `${width}${unit}`
}
