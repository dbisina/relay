// i18n.ts — a small, real localization layer, not a framework. Covers the
// highest-visibility surface: onboarding (where "pick your language" actually
// belongs), navigation, the home screen, and common words shared everywhere.
// Screens further from first-run (Providers, Pipelines detail forms, the
// Console) stay English-only for now — extending coverage means adding keys
// here and swapping literals for t('key') at the call sites, not new plumbing.

import { useEffect, useState } from 'react'

export type Locale = 'en' | 'es' | 'fr' | 'de' | 'ja' | 'zh' | 'pt'

export const LOCALES: { code: Locale; label: string }[] = [
  { code: 'en', label: 'English' },
  { code: 'es', label: 'Español' },
  { code: 'fr', label: 'Français' },
  { code: 'de', label: 'Deutsch' },
  { code: 'ja', label: '日本語' },
  { code: 'zh', label: '中文' },
  { code: 'pt', label: 'Português' },
]

type Dict = Record<string, string>

const en: Dict = {
  'nav.home': 'Home',
  'nav.detect': 'Scan & adopt',
  'nav.workflow': 'Agents & skills',
  'nav.pipelines': 'Pipelines',
  'nav.accounts': 'Accounts',
  'nav.providers': 'Providers',
  'nav.history': 'Time machine',
  'nav.settings': 'Settings',
  'nav.console': 'API console',

  'conn.up': 'Daemon connected',
  'conn.checking': 'Connecting',
  'conn.starting': 'Starting daemon',
  'conn.unreachable': 'Daemon offline',

  'home.welcome': 'Welcome back',
  'home.sessions': 'Sessions',
  'home.noSessions': 'No sessions yet. Describe a task below to run your first one.',
  'home.composerPlaceholder': 'Describe a task or ask a question',
  'home.scanAgents': 'Scan agents',
  'home.newProfile': 'New profile',
  'home.autoRotation': 'Auto rotation',
  'home.noProvidersEnabled': 'no providers enabled',

  'common.save': 'Save',
  'common.cancel': 'Cancel',
  'common.retry': 'Retry',
  'common.loading': 'Loading…',
  'common.close': 'Close',

  'onboarding.welcomeTitle': 'Welcome to Relay',
  'onboarding.welcomeBody':
    "Run a coding task once. When an agent hits its usage limit, Relay pauses it at a safe point, signs a continuation contract, and resumes on the next agent or account, so the work never stops for a copy-paste. A few quick steps to set up your rotation.",
  'onboarding.language': 'Language',
  'onboarding.getStarted': 'Get started',
  'onboarding.enableTitle': 'Enable the agents you have',
  'onboarding.enableBody':
    'Turn on the providers you can use. You can fine-tune and authenticate them later on the Providers screen.',
  'onboarding.accountTitle': 'Sign in to your first account',
  'onboarding.accountBody':
    "Relay launches the provider's own sign-in, it never sees your password. Add more accounts (personal, work, a second plan) anytime on the Accounts screen.",
  'onboarding.back': 'Back',
  'onboarding.next': 'Next',
  'onboarding.finish': 'Finish',
  'onboarding.skip': 'Skip setup',
  'onboarding.addAndSignIn': 'Add & sign in',
}

const es: Dict = {
  'nav.home': 'Inicio',
  'nav.detect': 'Buscar y adoptar',
  'nav.workflow': 'Agentes y skills',
  'nav.pipelines': 'Flujos',
  'nav.accounts': 'Cuentas',
  'nav.providers': 'Proveedores',
  'nav.history': 'Línea de tiempo',
  'nav.settings': 'Ajustes',
  'nav.console': 'Consola API',

  'conn.up': 'Daemon conectado',
  'conn.checking': 'Conectando',
  'conn.starting': 'Iniciando daemon',
  'conn.unreachable': 'Daemon desconectado',

  'home.welcome': 'Bienvenido de nuevo',
  'home.sessions': 'Sesiones',
  'home.noSessions': 'Aún no hay sesiones. Describe una tarea abajo para ejecutar la primera.',
  'home.composerPlaceholder': 'Describe una tarea o haz una pregunta',
  'home.scanAgents': 'Buscar agentes',
  'home.newProfile': 'Nuevo perfil',
  'home.autoRotation': 'Rotación automática',
  'home.noProvidersEnabled': 'sin proveedores habilitados',

  'common.save': 'Guardar',
  'common.cancel': 'Cancelar',
  'common.retry': 'Reintentar',
  'common.loading': 'Cargando…',
  'common.close': 'Cerrar',

  'onboarding.welcomeTitle': 'Bienvenido a Relay',
  'onboarding.welcomeBody':
    'Ejecuta una tarea una vez. Cuando un agente alcanza su límite de uso, Relay lo pausa en un punto seguro, firma un contrato de continuación y continúa con el siguiente agente o cuenta, para que el trabajo nunca se detenga por un copiar y pegar. Unos pasos rápidos para configurar tu rotación.',
  'onboarding.language': 'Idioma',
  'onboarding.getStarted': 'Comenzar',
  'onboarding.enableTitle': 'Habilita los agentes que tengas',
  'onboarding.enableBody':
    'Activa los proveedores que puedas usar. Puedes ajustarlos y autenticarlos después en la pantalla de Proveedores.',
  'onboarding.accountTitle': 'Inicia sesión en tu primera cuenta',
  'onboarding.accountBody':
    'Relay abre el inicio de sesión propio del proveedor, nunca ve tu contraseña. Añade más cuentas (personal, trabajo, otro plan) cuando quieras desde la pantalla de Cuentas.',
  'onboarding.back': 'Atrás',
  'onboarding.next': 'Siguiente',
  'onboarding.finish': 'Finalizar',
  'onboarding.skip': 'Omitir configuración',
  'onboarding.addAndSignIn': 'Añadir e iniciar sesión',
}

const fr: Dict = {
  'nav.home': 'Accueil',
  'nav.detect': 'Analyser et adopter',
  'nav.workflow': 'Agents et compétences',
  'nav.pipelines': 'Pipelines',
  'nav.accounts': 'Comptes',
  'nav.providers': 'Fournisseurs',
  'nav.history': 'Machine à remonter le temps',
  'nav.settings': 'Paramètres',
  'nav.console': 'Console API',

  'conn.up': 'Daemon connecté',
  'conn.checking': 'Connexion',
  'conn.starting': 'Démarrage du daemon',
  'conn.unreachable': 'Daemon hors ligne',

  'home.welcome': 'Content de vous revoir',
  'home.sessions': 'Sessions',
  'home.noSessions': "Aucune session pour l'instant. Décrivez une tâche ci-dessous pour lancer la première.",
  'home.composerPlaceholder': 'Décrivez une tâche ou posez une question',
  'home.scanAgents': 'Analyser les agents',
  'home.newProfile': 'Nouveau profil',
  'home.autoRotation': 'Rotation automatique',
  'home.noProvidersEnabled': 'aucun fournisseur activé',

  'common.save': 'Enregistrer',
  'common.cancel': 'Annuler',
  'common.retry': 'Réessayer',
  'common.loading': 'Chargement…',
  'common.close': 'Fermer',

  'onboarding.welcomeTitle': 'Bienvenue sur Relay',
  'onboarding.welcomeBody':
    "Lancez une tâche une seule fois. Quand un agent atteint sa limite d'utilisation, Relay le met en pause à un point sûr, signe un contrat de continuation, et reprend avec l'agent ou le compte suivant, pour que le travail ne s'arrête jamais pour un copier-coller. Quelques étapes rapides pour configurer votre rotation.",
  'onboarding.language': 'Langue',
  'onboarding.getStarted': 'Commencer',
  'onboarding.enableTitle': 'Activez les agents dont vous disposez',
  'onboarding.enableBody':
    "Activez les fournisseurs que vous pouvez utiliser. Vous pourrez les ajuster et les authentifier plus tard sur l'écran Fournisseurs.",
  'onboarding.accountTitle': 'Connectez-vous à votre premier compte',
  'onboarding.accountBody':
    "Relay lance la connexion propre au fournisseur, il ne voit jamais votre mot de passe. Ajoutez d'autres comptes (personnel, travail, un second forfait) à tout moment depuis l'écran Comptes.",
  'onboarding.back': 'Retour',
  'onboarding.next': 'Suivant',
  'onboarding.finish': 'Terminer',
  'onboarding.skip': 'Passer la configuration',
  'onboarding.addAndSignIn': 'Ajouter et se connecter',
}

const de: Dict = {
  'nav.home': 'Start',
  'nav.detect': 'Scannen & übernehmen',
  'nav.workflow': 'Agenten & Skills',
  'nav.pipelines': 'Pipelines',
  'nav.accounts': 'Konten',
  'nav.providers': 'Anbieter',
  'nav.history': 'Zeitmaschine',
  'nav.settings': 'Einstellungen',
  'nav.console': 'API-Konsole',

  'conn.up': 'Daemon verbunden',
  'conn.checking': 'Verbinde',
  'conn.starting': 'Daemon startet',
  'conn.unreachable': 'Daemon offline',

  'home.welcome': 'Willkommen zurück',
  'home.sessions': 'Sitzungen',
  'home.noSessions': 'Noch keine Sitzungen. Beschreibe unten eine Aufgabe, um die erste auszuführen.',
  'home.composerPlaceholder': 'Beschreibe eine Aufgabe oder stelle eine Frage',
  'home.scanAgents': 'Agenten scannen',
  'home.newProfile': 'Neues Profil',
  'home.autoRotation': 'Automatische Rotation',
  'home.noProvidersEnabled': 'keine Anbieter aktiviert',

  'common.save': 'Speichern',
  'common.cancel': 'Abbrechen',
  'common.retry': 'Erneut versuchen',
  'common.loading': 'Lädt…',
  'common.close': 'Schließen',

  'onboarding.welcomeTitle': 'Willkommen bei Relay',
  'onboarding.welcomeBody':
    'Starte eine Aufgabe einmal. Wenn ein Agent sein Nutzungslimit erreicht, pausiert Relay ihn an einem sicheren Punkt, signiert einen Fortsetzungsvertrag und setzt die Arbeit beim nächsten Agenten oder Konto fort, damit sie nie wegen Copy-Paste stoppt. Ein paar kurze Schritte für deine Rotation.',
  'onboarding.language': 'Sprache',
  'onboarding.getStarted': 'Loslegen',
  'onboarding.enableTitle': 'Aktiviere deine vorhandenen Agenten',
  'onboarding.enableBody':
    'Schalte die Anbieter ein, die du nutzen kannst. Feineinstellung und Authentifizierung kannst du später im Bildschirm Anbieter vornehmen.',
  'onboarding.accountTitle': 'Melde dich bei deinem ersten Konto an',
  'onboarding.accountBody':
    'Relay startet die eigene Anmeldung des Anbieters, es sieht niemals dein Passwort. Füge jederzeit weitere Konten (privat, Arbeit, ein zweiter Plan) im Bildschirm Konten hinzu.',
  'onboarding.back': 'Zurück',
  'onboarding.next': 'Weiter',
  'onboarding.finish': 'Fertig',
  'onboarding.skip': 'Einrichtung überspringen',
  'onboarding.addAndSignIn': 'Hinzufügen & anmelden',
}

const ja: Dict = {
  'nav.home': 'ホーム',
  'nav.detect': 'スキャン & 引き継ぎ',
  'nav.workflow': 'エージェントとスキル',
  'nav.pipelines': 'パイプライン',
  'nav.accounts': 'アカウント',
  'nav.providers': 'プロバイダー',
  'nav.history': 'タイムマシン',
  'nav.settings': '設定',
  'nav.console': 'API コンソール',

  'conn.up': 'デーモン接続済み',
  'conn.checking': '接続中',
  'conn.starting': 'デーモンを起動中',
  'conn.unreachable': 'デーモン未接続',

  'home.welcome': 'おかえりなさい',
  'home.sessions': 'セッション',
  'home.noSessions': 'セッションはまだありません。下でタスクを説明して最初の実行をしてください。',
  'home.composerPlaceholder': 'タスクを説明するか質問してください',
  'home.scanAgents': 'エージェントをスキャン',
  'home.newProfile': '新しいプロファイル',
  'home.autoRotation': '自動ローテーション',
  'home.noProvidersEnabled': '有効なプロバイダーがありません',

  'common.save': '保存',
  'common.cancel': 'キャンセル',
  'common.retry': '再試行',
  'common.loading': '読み込み中…',
  'common.close': '閉じる',

  'onboarding.welcomeTitle': 'Relay へようこそ',
  'onboarding.welcomeBody':
    'タスクを一度実行するだけ。エージェントが利用上限に達すると、Relay は安全な地点で一時停止し、継続契約に署名して、次のエージェントまたはアカウントで再開します。作業がコピー&ペーストで止まることはありません。ローテーションの設定はすぐに終わります。',
  'onboarding.language': '言語',
  'onboarding.getStarted': '始める',
  'onboarding.enableTitle': '使用するエージェントを有効にする',
  'onboarding.enableBody': '使用できるプロバイダーを有効にしてください。詳細設定と認証は後で「プロバイダー」画面で行えます。',
  'onboarding.accountTitle': '最初のアカウントにサインイン',
  'onboarding.accountBody':
    'Relay はプロバイダー自身のサインインを起動し、パスワードを見ることはありません。個人用、仕事用、別プランなどのアカウントは「アカウント」画面からいつでも追加できます。',
  'onboarding.back': '戻る',
  'onboarding.next': '次へ',
  'onboarding.finish': '完了',
  'onboarding.skip': '設定をスキップ',
  'onboarding.addAndSignIn': '追加してサインイン',
}

const zh: Dict = {
  'nav.home': '主页',
  'nav.detect': '扫描并接管',
  'nav.workflow': '代理与技能',
  'nav.pipelines': '流水线',
  'nav.accounts': '账户',
  'nav.providers': '提供商',
  'nav.history': '时间机器',
  'nav.settings': '设置',
  'nav.console': 'API 控制台',

  'conn.up': '守护进程已连接',
  'conn.checking': '正在连接',
  'conn.starting': '正在启动守护进程',
  'conn.unreachable': '守护进程离线',

  'home.welcome': '欢迎回来',
  'home.sessions': '会话',
  'home.noSessions': '还没有会话。在下面描述一个任务来运行第一个吧。',
  'home.composerPlaceholder': '描述一个任务或提出问题',
  'home.scanAgents': '扫描代理',
  'home.newProfile': '新建配置',
  'home.autoRotation': '自动轮换',
  'home.noProvidersEnabled': '没有已启用的提供商',

  'common.save': '保存',
  'common.cancel': '取消',
  'common.retry': '重试',
  'common.loading': '加载中…',
  'common.close': '关闭',

  'onboarding.welcomeTitle': '欢迎使用 Relay',
  'onboarding.welcomeBody':
    '只需运行一次任务。当某个代理达到使用上限时，Relay 会在安全点暂停它，签署一份延续合约，并在下一个代理或账户上恢复工作，让工作永不因手动复制粘贴而中断。只需几步即可完成轮换设置。',
  'onboarding.language': '语言',
  'onboarding.getStarted': '开始使用',
  'onboarding.enableTitle': '启用你拥有的代理',
  'onboarding.enableBody': '启用你可以使用的提供商。之后可以在“提供商”页面进行微调和身份验证。',
  'onboarding.accountTitle': '登录你的第一个账户',
  'onboarding.accountBody':
    'Relay 会启动提供商自己的登录流程，绝不会看到你的密码。随时可以在“账户”页面添加更多账户（个人、工作、第二个套餐）。',
  'onboarding.back': '返回',
  'onboarding.next': '下一步',
  'onboarding.finish': '完成',
  'onboarding.skip': '跳过设置',
  'onboarding.addAndSignIn': '添加并登录',
}

const pt: Dict = {
  'nav.home': 'Início',
  'nav.detect': 'Escanear e adotar',
  'nav.workflow': 'Agentes e skills',
  'nav.pipelines': 'Pipelines',
  'nav.accounts': 'Contas',
  'nav.providers': 'Provedores',
  'nav.history': 'Máquina do tempo',
  'nav.settings': 'Configurações',
  'nav.console': 'Console de API',

  'conn.up': 'Daemon conectado',
  'conn.checking': 'Conectando',
  'conn.starting': 'Iniciando daemon',
  'conn.unreachable': 'Daemon offline',

  'home.welcome': 'Bem-vindo de volta',
  'home.sessions': 'Sessões',
  'home.noSessions': 'Ainda não há sessões. Descreva uma tarefa abaixo para executar a primeira.',
  'home.composerPlaceholder': 'Descreva uma tarefa ou faça uma pergunta',
  'home.scanAgents': 'Escanear agentes',
  'home.newProfile': 'Novo perfil',
  'home.autoRotation': 'Rotação automática',
  'home.noProvidersEnabled': 'nenhum provedor habilitado',

  'common.save': 'Salvar',
  'common.cancel': 'Cancelar',
  'common.retry': 'Tentar novamente',
  'common.loading': 'Carregando…',
  'common.close': 'Fechar',

  'onboarding.welcomeTitle': 'Bem-vindo ao Relay',
  'onboarding.welcomeBody':
    'Execute uma tarefa uma vez. Quando um agente atinge seu limite de uso, o Relay o pausa em um ponto seguro, assina um contrato de continuação e retoma no próximo agente ou conta, para que o trabalho nunca pare por causa de copiar e colar. Alguns passos rápidos para configurar sua rotação.',
  'onboarding.language': 'Idioma',
  'onboarding.getStarted': 'Começar',
  'onboarding.enableTitle': 'Ative os agentes que você tem',
  'onboarding.enableBody':
    'Ative os provedores que você pode usar. Você pode ajustar e autenticar depois na tela Provedores.',
  'onboarding.accountTitle': 'Entre na sua primeira conta',
  'onboarding.accountBody':
    'O Relay abre o login próprio do provedor, ele nunca vê sua senha. Adicione mais contas (pessoal, trabalho, outro plano) a qualquer momento na tela Contas.',
  'onboarding.back': 'Voltar',
  'onboarding.next': 'Próximo',
  'onboarding.finish': 'Concluir',
  'onboarding.skip': 'Pular configuração',
  'onboarding.addAndSignIn': 'Adicionar e entrar',
}

const DICTS: Record<Locale, Dict> = { en, es, fr, de, ja, zh, pt }

function detectLocale(): Locale {
  try {
    const saved = localStorage.getItem('relay.locale') as Locale | null
    if (saved && DICTS[saved]) return saved
  } catch {
    /* localStorage unavailable */
  }
  const nav = (typeof navigator !== 'undefined' ? navigator.language : 'en').slice(0, 2)
  return (LOCALES.find((l) => l.code === nav)?.code as Locale) ?? 'en'
}

let current: Locale = detectLocale()
const listeners = new Set<() => void>()

export function getLocale(): Locale {
  return current
}
export function setLocale(l: Locale): void {
  current = l
  try {
    localStorage.setItem('relay.locale', l)
  } catch {
    /* ignore */
  }
  listeners.forEach((fn) => fn())
}
export function t(key: string): string {
  return DICTS[current][key] ?? DICTS.en[key] ?? key
}

/** Re-renders the calling component whenever the locale changes. */
export function useLocale(): { locale: Locale; t: (key: string) => string } {
  const [, tick] = useState(0)
  useEffect(() => {
    const fn = () => tick((x) => x + 1)
    listeners.add(fn)
    return () => {
      listeners.delete(fn)
    }
  }, [])
  return { locale: current, t }
}
