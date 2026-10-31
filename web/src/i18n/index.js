import { ref, watch } from 'vue'
import en from './en.json'
import ru from './ru.json'
import zh from './zh.json'

const dictionaries = { en, ru, zh }

export function normalizeLang(lang) {
  if (!lang || typeof lang !== 'string') return null
  const l = lang.trim().toLowerCase()
  if (l.startsWith('zh')) return 'zh'
  if (l.startsWith('ru')) return 'ru'
  if (l.startsWith('en')) return 'en'
  return null
}

function getHostLanguage() {
  // 1. Check URL parameters (?lang= or ?locale=)
  try {
    if (typeof window !== 'undefined' && window.location) {
      const params = new URLSearchParams(window.location.search)
      const q = params.get('lang') || params.get('locale')
      const normalized = normalizeLang(q)
      if (normalized) return normalized
    }
  } catch (e) {}

  // 2. Check user's manual preference in this plugin
  try {
    const userChoice = normalizeLang(localStorage.getItem('cpa_speedtest_user_lang'))
    if (userChoice) return userChoice
  } catch (e) {}

  // 3. Check CPA Host localStorage ('cli-proxy-language')
  try {
    const hostLangJson = localStorage.getItem('cli-proxy-language')
    if (hostLangJson) {
      const parsed = JSON.parse(hostLangJson)
      const normalized = normalizeLang(parsed?.state?.language)
      if (normalized) return normalized
    }
  } catch (e) {}

  // 4. Check same-origin parent window localStorage if inside iframe
  try {
    if (typeof window !== 'undefined' && window.parent && window.parent !== window) {
      const parentHostLang = window.parent.localStorage?.getItem('cli-proxy-language')
      if (parentHostLang) {
        const parsed = JSON.parse(parentHostLang)
        const normalized = normalizeLang(parsed?.state?.language)
        if (normalized) return normalized
      }
      const parentDocLang = normalizeLang(window.parent.document?.documentElement?.lang)
      if (parentDocLang) return parentDocLang
    }
  } catch (e) {}

  // 5. Fallback to browser language
  try {
    if (typeof navigator !== 'undefined' && navigator.language) {
      const browserLang = normalizeLang(navigator.language)
      if (browserLang) return browserLang
    }
  } catch (e) {}

  return 'en'
}

export const currentLang = ref(getHostLanguage())

// Sync with storage events from parent CPA dashboard or other tabs
if (typeof window !== 'undefined') {
  window.addEventListener('storage', (event) => {
    if (event.key === 'cli-proxy-language') {
      try {
        const parsed = JSON.parse(event.newValue || '{}')
        const normalized = normalizeLang(parsed?.state?.language)
        if (normalized && normalized in dictionaries) {
          currentLang.value = normalized
        }
      } catch (e) {}
    } else if (event.key === 'cpa_speedtest_user_lang') {
      const normalized = normalizeLang(event.newValue)
      if (normalized && normalized in dictionaries) {
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
