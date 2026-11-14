// 分组卡片样式和全局样式使用同一组数字：0 是详情大卡片，1 是小图标。
// 分组没有设置时跟随全局。选择器里的 -1 只存在于编辑界面，保存时写成空值。

export const groupIconStyleInfo = 0
export const groupIconStyleIcon = 1
export const groupIconStyleFollow = -1

export interface GroupIconStyleOption {
  label: string
  value: number
}

export function isStoredGroupIconStyle(value: unknown): value is 0 | 1 {
  return value === groupIconStyleInfo || value === groupIconStyleIcon
}

export function normalizeGroupIconStyle(value: unknown): 0 | 1 | null {
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value)
    return isStoredGroupIconStyle(parsed) ? parsed : null
  }
  return isStoredGroupIconStyle(value) ? value : null
}

export function resolveGroupIconStyle(groupStyle: unknown, globalStyle: unknown): 0 | 1 {
  return normalizeGroupIconStyle(groupStyle)
    ?? normalizeGroupIconStyle(globalStyle)
    ?? groupIconStyleIcon
}

export function groupIconStyleControlValue(value: unknown): number {
  return normalizeGroupIconStyle(value) ?? groupIconStyleFollow
}

export function groupIconStyleSelectOptions(label: (key: string) => string): GroupIconStyleOption[] {
  return [
    { label: label('apps.itemGroupManage.followGlobal'), value: groupIconStyleFollow },
    { label: label('apps.baseSettings.detailIcon'), value: groupIconStyleInfo },
    { label: label('apps.baseSettings.smallIcon'), value: groupIconStyleIcon },
  ]
}
