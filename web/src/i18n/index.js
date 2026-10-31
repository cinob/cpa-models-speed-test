import { computed } from 'vue'
import { useStorage } from '@vueuse/core'
import en from './en.json'
import ru from './ru.json'
import zh from './zh.json'

const dictionaries = { en, ru, zh }

// Detect user's preferred language or default to 'en'
function detectDefaultLang() {
  const browserLang = (typeof navigator !== 'undefined' && navigator.language) ? navigator.language.toLowerCase() : 'en'
  if (browserLang.startsWith('ru')) return 'ru'
  if (browserLang.startsWith('zh')) return 'zh'
  return 'en'
}

export const currentLang = useStorage('cpa_speedtest_lang', detectDefaultLang())

export function setLanguage(lang) {
  if (lang in dictionaries) {
    currentLang.value = lang
  }
}

export function t(path, params = {}) {
  const lang = (currentLang.value in dictionaries) ? currentLang.value : 'en'
  const keys = path.split('.')
  let val = dictionaries[lang]
  for (const k of keys) {
    if (val && typeof val === 'object' && k in val) {
      val = val[k]
    } else {
      // Fallback to English
      let fallback = dictionaries['en']
      for (const fk of keys) {
        if (fallback && typeof fallback === 'object' && fk in fallback) {
          fallback = fallback[fk]
        } else {
          return path
        }
      }
      val = fallback
      break
    }
  }
  if (typeof val !== 'string') return path
  return val.replace(/\{(\w+)\}/g, (_, k) => (k in params ? params[k] : `{${k}}`))
}
