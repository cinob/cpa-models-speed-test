import { ref } from 'vue'
import en from './en.json'
import ru from './ru.json'
import zhCN from './zh-CN.json'
import zhTW from './zh-TW.json'
import vi from './vi.json'
import ko from './ko.json'
import tr from './tr.json'

const dictionaries = {
  'en': en,
  'ru': ru,
  'zh-CN': zhCN,
  'zh-TW': zhTW,
  'vi': vi,
  'ko': ko,
  'tr': tr
}

export function normalizeLang(lang) {
  if (!lang || typeof lang !== 'string') return null
  const l = lang.trim().toLowerCase()
  if (l === 'zh-tw' || l.startsWith('zh-tw') || l.startsWith('zh-hk') || l.startsWith('zh-hant')) return 'zh-TW'
  if (l.startsWith('zh')) return 'zh-CN'
  if (l.startsWith('ru')) return 'ru'
  if (l.startsWith('vi')) return 'vi'
  if (l.startsWith('ko')) return 'ko'
  if (l.startsWith('tr')) return 'tr'
  if (l.startsWith('en')) return 'en'
  // Any other language falls back to null (which resolves to 'en')
  return null
}

function getHostLanguage() {
  // 1. Check URL parameters (?lang= or ?locale=)
  try {
    if (typeof window !== 'undefined' && window.location) {
      const params = new URLSearchParams(window.location.search)
      const q = params.get('lang') || params.get('locale')
      const normalized = normalizeLang(q)
      if (normalized && normalized in dictionaries) return normalized
    }
  } catch (e) {}

  // 2. Check user's manual selection in this plugin
  try {
    const userChoice = normalizeLang(localStorage.getItem('cpa_speedtest_user_lang'))
    if (userChoice && userChoice in dictionaries) return userChoice
  } catch (e) {}

  // 3. Check CPA Host localStorage ('cli-proxy-language')
  try {
    const hostLangJson = localStorage.getItem('cli-proxy-language')
    if (hostLangJson) {
      const parsed = JSON.parse(hostLangJson)
      const normalized = normalizeLang(parsed?.state?.language)
      if (normalized && normalized in dictionaries) return normalized
    }
  } catch (e) {}

  // 4. Check same-origin parent window localStorage if inside iframe
  try {
    if (typeof window !== 'undefined' && window.parent && window.parent !== window) {
      const parentHostLang = window.parent.localStorage?.getItem('cli-proxy-language')
      if (parentHostLang) {
        const parsed = JSON.parse(parentHostLang)
        const normalized = normalizeLang(parsed?.state?.language)
        if (normalized && normalized in dictionaries) return normalized
      }
      const parentDocLang = normalizeLang(window.parent.document?.documentElement?.lang)
      if (parentDocLang && parentDocLang in dictionaries) return parentDocLang
    }
  } catch (e) {}

  // Default is strictly English for any unsupported language or unset state
  return 'en'
}

export const currentLang = ref(getHostLanguage())

// Sync with storage events from parent CPA dashboard or other tabs
if (typeof window !== 'undefined') {
  window.addEventListener('storage', (event) => {
    if (event.key === 'cli-proxy-language') {
      try {
        const parsed = JSON.parse(event.newValue || '{}')
        const normalized = normalizeLang(parsed?.state?.language) || 'en'
        if (normalized in dictionaries) {
          currentLang.value = normalized
        }
      } catch (e) {}
    } else if (event.key === 'cpa_speedtest_user_lang') {
      const normalized = normalizeLang(event.newValue) || 'en'
      if (normalized in dictionaries) {
        currentLang.value = normalized
      }
    }
  })

  // Listen to postMessage from parent iframe host
  window.addEventListener('message', (event) => {
    if (event.data && typeof event.data === 'object') {
      const msgLang = normalizeLang(event.data.language || event.data.lang)
      if (msgLang && msgLang in dictionaries) {
        currentLang.value = msgLang
      }
    }
  })
}

export function setLanguage(lang) {
  const normalized = normalizeLang(lang) || 'en'
  if (normalized in dictionaries) {
    currentLang.value = normalized
    try {
      localStorage.setItem('cpa_speedtest_user_lang', normalized)
      localStorage.setItem('cpa_speedtest_lang', normalized)
    } catch (e) {}
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
