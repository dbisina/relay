// i18n.ts — a small, real localization layer, not a framework. Covers the
// highest-visibility surface: onboarding (where "pick your language" actually
// belongs), navigation, the home screen, and common words shared everywhere.
// Screens further from first-run (Providers, Pipelines detail forms, the
// Console) stay English-only for now — extending coverage means adding keys
// here and swapping literals for t('key') at the call sites, not new plumbing.
// Direction is part of the locale, not a separate setting: setLocale flips
// document.dir so a right-to-left language mirrors the app on its own.

import { useEffect, useState } from 'react'

export type Locale =
  | 'en'
  | 'es'
  | 'fr'
  | 'de'
  | 'ja'
  | 'zh'
  | 'pt'
  | 'hi'
  | 'ar'
  | 'ru'
  | 'id'
  | 'ko'
  | 'it'
  | 'tr'
  | 'vi'
  | 'bn'
  | 'sw'

// Labels are endonyms: a language is easiest to find written in itself. `rtl`
// drives document direction, so a right-to-left locale flips the whole layout
// through the CSS `dir` cascade rather than through per-component overrides.
export const LOCALES: { code: Locale; label: string; rtl: boolean }[] = [
  { code: 'en', label: 'English', rtl: false },
  { code: 'es', label: 'Español', rtl: false },
  { code: 'fr', label: 'Français', rtl: false },
  { code: 'de', label: 'Deutsch', rtl: false },
  { code: 'ja', label: '日本語', rtl: false },
  { code: 'zh', label: '中文', rtl: false },
  { code: 'pt', label: 'Português', rtl: false },
  { code: 'hi', label: 'हिन्दी', rtl: false },
  { code: 'ar', label: 'العربية', rtl: true },
  { code: 'ru', label: 'Русский', rtl: false },
  { code: 'id', label: 'Bahasa Indonesia', rtl: false },
  { code: 'ko', label: '한국어', rtl: false },
  { code: 'it', label: 'Italiano', rtl: false },
  { code: 'tr', label: 'Türkçe', rtl: false },
  { code: 'vi', label: 'Tiếng Việt', rtl: false },
  { code: 'bn', label: 'বাংলা', rtl: false },
  { code: 'sw', label: 'Kiswahili', rtl: false },
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

const hi: Dict = {
  'nav.home': 'होम',
  'nav.detect': 'स्कैन करें और अपनाएँ',
  'nav.workflow': 'एजेंट और स्किल',
  'nav.pipelines': 'पाइपलाइन',
  'nav.accounts': 'खाते',
  'nav.providers': 'प्रोवाइडर',
  'nav.history': 'टाइम मशीन',
  'nav.settings': 'सेटिंग्स',
  'nav.console': 'API कंसोल',

  'conn.up': 'डीमन जुड़ा है',
  'conn.checking': 'कनेक्ट हो रहा है',
  'conn.starting': 'डीमन शुरू हो रहा है',
  'conn.unreachable': 'डीमन ऑफ़लाइन',

  'home.welcome': 'वापसी पर स्वागत है',
  'home.sessions': 'सेशन',
  'home.noSessions': 'अभी कोई सेशन नहीं है। पहला चलाने के लिए नीचे कोई काम बताएँ।',
  'home.composerPlaceholder': 'कोई काम बताएँ या सवाल पूछें',
  'home.scanAgents': 'एजेंट स्कैन करें',
  'home.newProfile': 'नई प्रोफ़ाइल',
  'home.autoRotation': 'ऑटो रोटेशन',
  'home.noProvidersEnabled': 'कोई प्रोवाइडर चालू नहीं',

  'common.save': 'सहेजें',
  'common.cancel': 'रद्द करें',
  'common.retry': 'फिर कोशिश करें',
  'common.loading': 'लोड हो रहा है…',
  'common.close': 'बंद करें',

  'onboarding.welcomeTitle': 'Relay में आपका स्वागत है',
  'onboarding.welcomeBody':
    'कोडिंग का काम एक बार शुरू करें। जब कोई एजेंट अपनी उपयोग सीमा पर पहुँचता है, Relay उसे सुरक्षित जगह पर रोकता है, एक कंटिन्यूएशन कॉन्ट्रैक्ट पर हस्ताक्षर करता है और अगले एजेंट या खाते पर काम आगे बढ़ाता है, ताकि कॉपी पेस्ट के लिए काम कभी न रुके। अपना रोटेशन सेट करने के लिए कुछ आसान कदम।',
  'onboarding.language': 'भाषा',
  'onboarding.getStarted': 'शुरू करें',
  'onboarding.enableTitle': 'आपके पास जो एजेंट हैं उन्हें चालू करें',
  'onboarding.enableBody':
    'जिन प्रोवाइडर का आप उपयोग कर सकते हैं उन्हें चालू करें। बारीक सेटिंग और साइन इन बाद में प्रोवाइडर स्क्रीन पर कर सकते हैं।',
  'onboarding.accountTitle': 'अपने पहले खाते में साइन इन करें',
  'onboarding.accountBody':
    'Relay प्रोवाइडर का अपना साइन इन खोलता है, वह आपका पासवर्ड कभी नहीं देखता। निजी, ऑफ़िस या दूसरा प्लान, और खाते आप कभी भी खाते स्क्रीन से जोड़ सकते हैं।',
  'onboarding.back': 'पीछे',
  'onboarding.next': 'आगे',
  'onboarding.finish': 'पूरा करें',
  'onboarding.skip': 'सेटअप छोड़ें',
  'onboarding.addAndSignIn': 'जोड़ें और साइन इन करें',
}

const ar: Dict = {
  'nav.home': 'الرئيسية',
  'nav.detect': 'فحص واعتماد',
  'nav.workflow': 'الوكلاء والمهارات',
  'nav.pipelines': 'المسارات',
  'nav.accounts': 'الحسابات',
  'nav.providers': 'المزوّدون',
  'nav.history': 'آلة الزمن',
  'nav.settings': 'الإعدادات',
  'nav.console': 'وحدة تحكم API',

  'conn.up': 'الديمون متصل',
  'conn.checking': 'جارٍ الاتصال',
  'conn.starting': 'جارٍ تشغيل الديمون',
  'conn.unreachable': 'الديمون غير متصل',

  'home.welcome': 'أهلاً بعودتك',
  'home.sessions': 'الجلسات',
  'home.noSessions': 'لا توجد جلسات بعد. صف مهمة في الأسفل لتشغيل أول جلسة.',
  'home.composerPlaceholder': 'صف مهمة أو اطرح سؤالاً',
  'home.scanAgents': 'فحص الوكلاء',
  'home.newProfile': 'ملف جديد',
  'home.autoRotation': 'تناوب تلقائي',
  'home.noProvidersEnabled': 'لا يوجد مزوّدون مفعّلون',

  'common.save': 'حفظ',
  'common.cancel': 'إلغاء',
  'common.retry': 'إعادة المحاولة',
  'common.loading': 'جارٍ التحميل…',
  'common.close': 'إغلاق',

  'onboarding.welcomeTitle': 'مرحباً بك في Relay',
  'onboarding.welcomeBody':
    'شغّل مهمة برمجية مرة واحدة. عندما يصل أحد الوكلاء إلى حد الاستخدام، يوقفه Relay عند نقطة آمنة، ويوقّع عقد استكمال، ثم يتابع العمل على الوكيل أو الحساب التالي، حتى لا يتوقف العمل من أجل نسخ ولصق. خطوات سريعة لإعداد التناوب.',
  'onboarding.language': 'اللغة',
  'onboarding.getStarted': 'ابدأ',
  'onboarding.enableTitle': 'فعّل الوكلاء المتوفرين لديك',
  'onboarding.enableBody':
    'فعّل المزوّدين الذين يمكنك استخدامهم. يمكنك ضبطهم وتسجيل الدخول إليهم لاحقاً من شاشة المزوّدين.',
  'onboarding.accountTitle': 'سجّل الدخول إلى حسابك الأول',
  'onboarding.accountBody':
    'يفتح Relay صفحة تسجيل الدخول الخاصة بالمزوّد، وهو لا يرى كلمة مرورك أبداً. أضف حسابات أخرى، شخصية أو للعمل أو باشتراك ثانٍ، في أي وقت من شاشة الحسابات.',
  'onboarding.back': 'رجوع',
  'onboarding.next': 'التالي',
  'onboarding.finish': 'إنهاء',
  'onboarding.skip': 'تخطي الإعداد',
  'onboarding.addAndSignIn': 'إضافة وتسجيل الدخول',
}

const ru: Dict = {
  'nav.home': 'Главная',
  'nav.detect': 'Сканировать и подключить',
  'nav.workflow': 'Агенты и навыки',
  'nav.pipelines': 'Конвейеры',
  'nav.accounts': 'Аккаунты',
  'nav.providers': 'Провайдеры',
  'nav.history': 'Машина времени',
  'nav.settings': 'Настройки',
  'nav.console': 'Консоль API',

  'conn.up': 'Демон подключён',
  'conn.checking': 'Подключение',
  'conn.starting': 'Запуск демона',
  'conn.unreachable': 'Демон недоступен',

  'home.welcome': 'С возвращением',
  'home.sessions': 'Сессии',
  'home.noSessions': 'Сессий пока нет. Опишите задачу ниже, чтобы запустить первую.',
  'home.composerPlaceholder': 'Опишите задачу или задайте вопрос',
  'home.scanAgents': 'Сканировать агентов',
  'home.newProfile': 'Новый профиль',
  'home.autoRotation': 'Автоматическая ротация',
  'home.noProvidersEnabled': 'нет включённых провайдеров',

  'common.save': 'Сохранить',
  'common.cancel': 'Отмена',
  'common.retry': 'Повторить',
  'common.loading': 'Загрузка…',
  'common.close': 'Закрыть',

  'onboarding.welcomeTitle': 'Добро пожаловать в Relay',
  'onboarding.welcomeBody':
    'Запустите задачу один раз. Когда агент достигает лимита использования, Relay ставит его на паузу в безопасной точке, подписывает контракт продолжения и продолжает работу на следующем агенте или аккаунте, чтобы дело никогда не останавливалось ради копирования и вставки. Несколько быстрых шагов, чтобы настроить ротацию.',
  'onboarding.language': 'Язык',
  'onboarding.getStarted': 'Начать',
  'onboarding.enableTitle': 'Включите агентов, которые у вас есть',
  'onboarding.enableBody':
    'Включите провайдеров, которыми можете пользоваться. Настроить и авторизовать их можно позже на экране «Провайдеры».',
  'onboarding.accountTitle': 'Войдите в первый аккаунт',
  'onboarding.accountBody':
    'Relay открывает собственную страницу входа провайдера и никогда не видит ваш пароль. Другие аккаунты, личный, рабочий или второй тариф, можно добавить в любой момент на экране «Аккаунты».',
  'onboarding.back': 'Назад',
  'onboarding.next': 'Далее',
  'onboarding.finish': 'Готово',
  'onboarding.skip': 'Пропустить настройку',
  'onboarding.addAndSignIn': 'Добавить и войти',
}

const id: Dict = {
  'nav.home': 'Beranda',
  'nav.detect': 'Pindai & adopsi',
  'nav.workflow': 'Agen & skill',
  'nav.pipelines': 'Pipeline',
  'nav.accounts': 'Akun',
  'nav.providers': 'Penyedia',
  'nav.history': 'Mesin waktu',
  'nav.settings': 'Pengaturan',
  'nav.console': 'Konsol API',

  'conn.up': 'Daemon terhubung',
  'conn.checking': 'Menghubungkan',
  'conn.starting': 'Menjalankan daemon',
  'conn.unreachable': 'Daemon offline',

  'home.welcome': 'Selamat datang kembali',
  'home.sessions': 'Sesi',
  'home.noSessions': 'Belum ada sesi. Jelaskan sebuah tugas di bawah untuk menjalankan yang pertama.',
  'home.composerPlaceholder': 'Jelaskan tugas atau ajukan pertanyaan',
  'home.scanAgents': 'Pindai agen',
  'home.newProfile': 'Profil baru',
  'home.autoRotation': 'Rotasi otomatis',
  'home.noProvidersEnabled': 'tidak ada penyedia yang aktif',

  'common.save': 'Simpan',
  'common.cancel': 'Batal',
  'common.retry': 'Coba lagi',
  'common.loading': 'Memuat…',
  'common.close': 'Tutup',

  'onboarding.welcomeTitle': 'Selamat datang di Relay',
  'onboarding.welcomeBody':
    'Jalankan satu tugas pemrograman sekali saja. Saat sebuah agen mencapai batas pemakaiannya, Relay menjedanya di titik yang aman, menandatangani kontrak lanjutan, lalu meneruskan pekerjaan ke agen atau akun berikutnya, sehingga pekerjaan tidak pernah berhenti hanya karena salin tempel. Beberapa langkah singkat untuk menyiapkan rotasi Anda.',
  'onboarding.language': 'Bahasa',
  'onboarding.getStarted': 'Mulai',
  'onboarding.enableTitle': 'Aktifkan agen yang Anda miliki',
  'onboarding.enableBody':
    'Aktifkan penyedia yang bisa Anda pakai. Anda dapat menyetelnya dan masuk ke akunnya nanti di layar Penyedia.',
  'onboarding.accountTitle': 'Masuk ke akun pertama Anda',
  'onboarding.accountBody':
    'Relay membuka halaman masuk milik penyedia itu sendiri, jadi kata sandi Anda tidak pernah terlihat. Tambahkan akun lain, pribadi, kantor, atau paket kedua, kapan saja di layar Akun.',
  'onboarding.back': 'Kembali',
  'onboarding.next': 'Lanjut',
  'onboarding.finish': 'Selesai',
  'onboarding.skip': 'Lewati penyiapan',
  'onboarding.addAndSignIn': 'Tambah & masuk',
}

const ko: Dict = {
  'nav.home': '홈',
  'nav.detect': '스캔 및 가져오기',
  'nav.workflow': '에이전트 및 스킬',
  'nav.pipelines': '파이프라인',
  'nav.accounts': '계정',
  'nav.providers': '프로바이더',
  'nav.history': '타임머신',
  'nav.settings': '설정',
  'nav.console': 'API 콘솔',

  'conn.up': '데몬 연결됨',
  'conn.checking': '연결 중',
  'conn.starting': '데몬 시작 중',
  'conn.unreachable': '데몬 오프라인',

  'home.welcome': '다시 오신 것을 환영합니다',
  'home.sessions': '세션',
  'home.noSessions': '아직 세션이 없습니다. 아래에 작업을 설명해 첫 세션을 실행해 보세요.',
  'home.composerPlaceholder': '작업을 설명하거나 질문을 입력하세요',
  'home.scanAgents': '에이전트 스캔',
  'home.newProfile': '새 프로필',
  'home.autoRotation': '자동 로테이션',
  'home.noProvidersEnabled': '활성화된 프로바이더 없음',

  'common.save': '저장',
  'common.cancel': '취소',
  'common.retry': '다시 시도',
  'common.loading': '불러오는 중…',
  'common.close': '닫기',

  'onboarding.welcomeTitle': 'Relay에 오신 것을 환영합니다',
  'onboarding.welcomeBody':
    '코딩 작업을 한 번만 실행하세요. 에이전트가 사용 한도에 도달하면 Relay가 안전한 지점에서 작업을 멈추고 연속 계약에 서명한 뒤 다음 에이전트나 계정에서 이어서 진행합니다. 복사와 붙여넣기 때문에 작업이 멈추는 일은 없습니다. 몇 단계만 거치면 로테이션 설정이 끝납니다.',
  'onboarding.language': '언어',
  'onboarding.getStarted': '시작하기',
  'onboarding.enableTitle': '보유한 에이전트를 활성화하세요',
  'onboarding.enableBody':
    '사용할 수 있는 프로바이더를 켜세요. 세부 설정과 인증은 나중에 프로바이더 화면에서 할 수 있습니다.',
  'onboarding.accountTitle': '첫 번째 계정에 로그인하세요',
  'onboarding.accountBody':
    'Relay는 프로바이더 자체 로그인 화면을 띄우며 비밀번호를 보지 않습니다. 개인, 업무, 두 번째 요금제 등 계정은 언제든지 계정 화면에서 추가할 수 있습니다.',
  'onboarding.back': '뒤로',
  'onboarding.next': '다음',
  'onboarding.finish': '완료',
  'onboarding.skip': '설정 건너뛰기',
  'onboarding.addAndSignIn': '추가하고 로그인',
}

const it: Dict = {
  'nav.home': 'Home',
  'nav.detect': 'Cerca e adotta',
  'nav.workflow': 'Agenti e skill',
  'nav.pipelines': 'Pipeline',
  'nav.accounts': 'Account',
  'nav.providers': 'Provider',
  'nav.history': 'Macchina del tempo',
  'nav.settings': 'Impostazioni',
  'nav.console': 'Console API',

  'conn.up': 'Daemon connesso',
  'conn.checking': 'Connessione',
  'conn.starting': 'Avvio del daemon',
  'conn.unreachable': 'Daemon offline',

  'home.welcome': 'Bentornato',
  'home.sessions': 'Sessioni',
  'home.noSessions': 'Ancora nessuna sessione. Descrivi un compito qui sotto per eseguire la prima.',
  'home.composerPlaceholder': 'Descrivi un compito o fai una domanda',
  'home.scanAgents': 'Cerca agenti',
  'home.newProfile': 'Nuovo profilo',
  'home.autoRotation': 'Rotazione automatica',
  'home.noProvidersEnabled': 'nessun provider attivo',

  'common.save': 'Salva',
  'common.cancel': 'Annulla',
  'common.retry': 'Riprova',
  'common.loading': 'Caricamento…',
  'common.close': 'Chiudi',

  'onboarding.welcomeTitle': 'Benvenuto in Relay',
  'onboarding.welcomeBody':
    "Avvia un compito di programmazione una sola volta. Quando un agente raggiunge il suo limite di utilizzo, Relay lo mette in pausa in un punto sicuro, firma un contratto di continuazione e riprende sull'agente o sull'account successivo, così il lavoro non si ferma mai per un copia e incolla. Bastano pochi passaggi per configurare la rotazione.",
  'onboarding.language': 'Lingua',
  'onboarding.getStarted': 'Inizia',
  'onboarding.enableTitle': 'Attiva gli agenti che hai',
  'onboarding.enableBody':
    'Attiva i provider che puoi usare. Potrai regolarli e autenticarli più tardi nella schermata Provider.',
  'onboarding.accountTitle': 'Accedi al tuo primo account',
  'onboarding.accountBody':
    "Relay apre la pagina di accesso del provider, non vede mai la tua password. Aggiungi altri account, personale, di lavoro o un secondo piano, quando vuoi dalla schermata Account.",
  'onboarding.back': 'Indietro',
  'onboarding.next': 'Avanti',
  'onboarding.finish': 'Fine',
  'onboarding.skip': 'Salta la configurazione',
  'onboarding.addAndSignIn': 'Aggiungi e accedi',
}

const tr: Dict = {
  'nav.home': 'Ana sayfa',
  'nav.detect': 'Tara ve devral',
  'nav.workflow': 'Ajanlar ve beceriler',
  'nav.pipelines': 'İş hatları',
  'nav.accounts': 'Hesaplar',
  'nav.providers': 'Sağlayıcılar',
  'nav.history': 'Zaman makinesi',
  'nav.settings': 'Ayarlar',
  'nav.console': 'API konsolu',

  'conn.up': 'Daemon bağlı',
  'conn.checking': 'Bağlanıyor',
  'conn.starting': 'Daemon başlatılıyor',
  'conn.unreachable': 'Daemon çevrimdışı',

  'home.welcome': 'Tekrar hoş geldiniz',
  'home.sessions': 'Oturumlar',
  'home.noSessions': 'Henüz oturum yok. İlkini çalıştırmak için aşağıda bir görev anlatın.',
  'home.composerPlaceholder': 'Bir görev anlatın ya da soru sorun',
  'home.scanAgents': 'Ajanları tara',
  'home.newProfile': 'Yeni profil',
  'home.autoRotation': 'Otomatik rotasyon',
  'home.noProvidersEnabled': 'etkin sağlayıcı yok',

  'common.save': 'Kaydet',
  'common.cancel': 'Vazgeç',
  'common.retry': 'Yeniden dene',
  'common.loading': 'Yükleniyor…',
  'common.close': 'Kapat',

  'onboarding.welcomeTitle': "Relay'e hoş geldiniz",
  'onboarding.welcomeBody':
    'Bir kodlama görevini bir kez başlatın. Bir ajan kullanım sınırına ulaştığında Relay onu güvenli bir noktada duraklatır, bir devam sözleşmesi imzalar ve sıradaki ajan ya da hesapta işi sürdürür, böylece iş kopyala yapıştır yüzünden hiç durmaz. Rotasyonunuzu kurmak için birkaç hızlı adım.',
  'onboarding.language': 'Dil',
  'onboarding.getStarted': 'Başla',
  'onboarding.enableTitle': 'Sahip olduğunuz ajanları etkinleştirin',
  'onboarding.enableBody':
    'Kullanabileceğiniz sağlayıcıları açın. İnce ayarları ve oturum açmayı daha sonra Sağlayıcılar ekranından yapabilirsiniz.',
  'onboarding.accountTitle': 'İlk hesabınızda oturum açın',
  'onboarding.accountBody':
    'Relay, sağlayıcının kendi oturum açma ekranını açar, parolanızı asla görmez. Kişisel, iş ya da ikinci bir plan, istediğiniz zaman Hesaplar ekranından yeni hesap ekleyebilirsiniz.',
  'onboarding.back': 'Geri',
  'onboarding.next': 'İleri',
  'onboarding.finish': 'Bitir',
  'onboarding.skip': 'Kurulumu atla',
  'onboarding.addAndSignIn': 'Ekle ve oturum aç',
}

const vi: Dict = {
  'nav.home': 'Trang chủ',
  'nav.detect': 'Quét và tiếp nhận',
  'nav.workflow': 'Tác nhân và kỹ năng',
  'nav.pipelines': 'Chuỗi xử lý',
  'nav.accounts': 'Tài khoản',
  'nav.providers': 'Nhà cung cấp',
  'nav.history': 'Cỗ máy thời gian',
  'nav.settings': 'Cài đặt',
  'nav.console': 'Bảng điều khiển API',

  'conn.up': 'Daemon đã kết nối',
  'conn.checking': 'Đang kết nối',
  'conn.starting': 'Đang khởi động daemon',
  'conn.unreachable': 'Daemon ngoại tuyến',

  'home.welcome': 'Chào mừng trở lại',
  'home.sessions': 'Phiên làm việc',
  'home.noSessions': 'Chưa có phiên nào. Hãy mô tả một công việc bên dưới để chạy phiên đầu tiên.',
  'home.composerPlaceholder': 'Mô tả một công việc hoặc đặt câu hỏi',
  'home.scanAgents': 'Quét tác nhân',
  'home.newProfile': 'Hồ sơ mới',
  'home.autoRotation': 'Luân phiên tự động',
  'home.noProvidersEnabled': 'chưa bật nhà cung cấp nào',

  'common.save': 'Lưu',
  'common.cancel': 'Hủy',
  'common.retry': 'Thử lại',
  'common.loading': 'Đang tải…',
  'common.close': 'Đóng',

  'onboarding.welcomeTitle': 'Chào mừng bạn đến với Relay',
  'onboarding.welcomeBody':
    'Chạy một công việc lập trình một lần duy nhất. Khi một tác nhân chạm giới hạn sử dụng, Relay tạm dừng nó ở điểm an toàn, ký một hợp đồng tiếp nối, rồi tiếp tục trên tác nhân hoặc tài khoản kế tiếp, để công việc không bao giờ dừng lại chỉ vì sao chép và dán. Vài bước nhanh để thiết lập luân phiên của bạn.',
  'onboarding.language': 'Ngôn ngữ',
  'onboarding.getStarted': 'Bắt đầu',
  'onboarding.enableTitle': 'Bật những tác nhân bạn đang có',
  'onboarding.enableBody':
    'Bật những nhà cung cấp bạn có thể dùng. Bạn có thể tinh chỉnh và đăng nhập sau ở màn hình Nhà cung cấp.',
  'onboarding.accountTitle': 'Đăng nhập tài khoản đầu tiên',
  'onboarding.accountBody':
    'Relay mở trang đăng nhập của chính nhà cung cấp, nó không bao giờ thấy mật khẩu của bạn. Thêm tài khoản khác, cá nhân, công việc hay một gói thứ hai, bất cứ lúc nào ở màn hình Tài khoản.',
  'onboarding.back': 'Quay lại',
  'onboarding.next': 'Tiếp theo',
  'onboarding.finish': 'Hoàn tất',
  'onboarding.skip': 'Bỏ qua thiết lập',
  'onboarding.addAndSignIn': 'Thêm và đăng nhập',
}

const bn: Dict = {
  'nav.home': 'হোম',
  'nav.detect': 'স্ক্যান ও গ্রহণ',
  'nav.workflow': 'এজেন্ট ও স্কিল',
  'nav.pipelines': 'পাইপলাইন',
  'nav.accounts': 'অ্যাকাউন্ট',
  'nav.providers': 'প্রোভাইডার',
  'nav.history': 'টাইম মেশিন',
  'nav.settings': 'সেটিংস',
  'nav.console': 'API কনসোল',

  'conn.up': 'ডেমন যুক্ত আছে',
  'conn.checking': 'সংযোগ করা হচ্ছে',
  'conn.starting': 'ডেমন চালু হচ্ছে',
  'conn.unreachable': 'ডেমন অফলাইন',

  'home.welcome': 'আবার স্বাগতম',
  'home.sessions': 'সেশন',
  'home.noSessions': 'এখনও কোনো সেশন নেই। প্রথমটি চালাতে নিচে একটি কাজ লিখুন।',
  'home.composerPlaceholder': 'একটি কাজ বর্ণনা করুন বা প্রশ্ন করুন',
  'home.scanAgents': 'এজেন্ট স্ক্যান করুন',
  'home.newProfile': 'নতুন প্রোফাইল',
  'home.autoRotation': 'স্বয়ংক্রিয় রোটেশন',
  'home.noProvidersEnabled': 'কোনো প্রোভাইডার চালু নেই',

  'common.save': 'সংরক্ষণ',
  'common.cancel': 'বাতিল',
  'common.retry': 'আবার চেষ্টা',
  'common.loading': 'লোড হচ্ছে…',
  'common.close': 'বন্ধ',

  'onboarding.welcomeTitle': 'Relay-তে স্বাগতম',
  'onboarding.welcomeBody':
    'একবারই কোডিংয়ের কাজ চালু করুন। কোনো এজেন্ট তার ব্যবহারের সীমায় পৌঁছালে Relay সেটিকে নিরাপদ জায়গায় থামায়, একটি ধারাবাহিকতা চুক্তিতে স্বাক্ষর করে এবং পরের এজেন্ট বা অ্যাকাউন্টে কাজ চালিয়ে যায়, যাতে কপি পেস্টের জন্য কাজ কখনও না থামে। আপনার রোটেশন সেট করতে কয়েকটি দ্রুত ধাপ।',
  'onboarding.language': 'ভাষা',
  'onboarding.getStarted': 'শুরু করুন',
  'onboarding.enableTitle': 'আপনার কাছে থাকা এজেন্টগুলো চালু করুন',
  'onboarding.enableBody':
    'যে প্রোভাইডারগুলো ব্যবহার করতে পারবেন সেগুলো চালু করুন। খুঁটিনাটি সেটিং ও সাইন ইন পরে প্রোভাইডার স্ক্রিনে করা যাবে।',
  'onboarding.accountTitle': 'আপনার প্রথম অ্যাকাউন্টে সাইন ইন করুন',
  'onboarding.accountBody':
    'Relay প্রোভাইডারের নিজস্ব সাইন ইন খোলে, এটি কখনও আপনার পাসওয়ার্ড দেখে না। ব্যক্তিগত, অফিস বা দ্বিতীয় প্ল্যান, যেকোনো সময় অ্যাকাউন্ট স্ক্রিন থেকে আরও অ্যাকাউন্ট যোগ করুন।',
  'onboarding.back': 'পেছনে',
  'onboarding.next': 'পরবর্তী',
  'onboarding.finish': 'শেষ করুন',
  'onboarding.skip': 'সেটআপ বাদ দিন',
  'onboarding.addAndSignIn': 'যোগ করে সাইন ইন করুন',
}

const sw: Dict = {
  'nav.home': 'Mwanzo',
  'nav.detect': 'Changanua na tumia',
  'nav.workflow': 'Mawakala na ujuzi',
  'nav.pipelines': 'Mifululizo',
  'nav.accounts': 'Akaunti',
  'nav.providers': 'Watoa huduma',
  'nav.history': 'Mashine ya wakati',
  'nav.settings': 'Mipangilio',
  'nav.console': 'Konsoli ya API',

  'conn.up': 'Daemon imeunganishwa',
  'conn.checking': 'Inaunganisha',
  'conn.starting': 'Inaanzisha daemon',
  'conn.unreachable': 'Daemon haipatikani',

  'home.welcome': 'Karibu tena',
  'home.sessions': 'Vipindi',
  'home.noSessions': 'Bado hakuna vipindi. Eleza kazi hapa chini ili kuendesha kipindi cha kwanza.',
  'home.composerPlaceholder': 'Eleza kazi au uliza swali',
  'home.scanAgents': 'Changanua mawakala',
  'home.newProfile': 'Wasifu mpya',
  'home.autoRotation': 'Mzunguko wa kiotomatiki',
  'home.noProvidersEnabled': 'hakuna mtoa huduma aliyewashwa',

  'common.save': 'Hifadhi',
  'common.cancel': 'Ghairi',
  'common.retry': 'Jaribu tena',
  'common.loading': 'Inapakia…',
  'common.close': 'Funga',

  'onboarding.welcomeTitle': 'Karibu Relay',
  'onboarding.welcomeBody':
    'Endesha kazi ya uandishi wa msimbo mara moja tu. Wakala anapofika kikomo cha matumizi, Relay humsimamisha mahali salama, husaini mkataba wa kuendeleza kazi, na kuendelea na wakala au akaunti inayofuata, ili kazi isikwame kwa sababu ya kunakili na kubandika. Hatua chache tu za kuweka mzunguko wako.',
  'onboarding.language': 'Lugha',
  'onboarding.getStarted': 'Anza',
  'onboarding.enableTitle': 'Washa mawakala ulio nao',
  'onboarding.enableBody':
    'Washa watoa huduma unaoweza kutumia. Unaweza kuwarekebisha na kuingia baadaye kwenye skrini ya Watoa huduma.',
  'onboarding.accountTitle': 'Ingia kwenye akaunti yako ya kwanza',
  'onboarding.accountBody':
    'Relay hufungua ukurasa wa kuingia wa mtoa huduma mwenyewe, haioni nenosiri lako kamwe. Ongeza akaunti nyingine, ya binafsi, ya kazini, au mpango wa pili, wakati wowote kwenye skrini ya Akaunti.',
  'onboarding.back': 'Rudi',
  'onboarding.next': 'Endelea',
  'onboarding.finish': 'Maliza',
  'onboarding.skip': 'Ruka usanidi',
  'onboarding.addAndSignIn': 'Ongeza na uingie',
}

const DICTS: Record<Locale, Dict> = {
  en,
  es,
  fr,
  de,
  ja,
  zh,
  pt,
  hi,
  ar,
  ru,
  id,
  ko,
  it,
  tr,
  vi,
  bn,
  sw,
}

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

// Right-to-left scripts. A set rather than a flag on every dict so the metadata
// stays in LOCALES, where the language picker already reads it.
const RTL_LOCALES: ReadonlySet<Locale> = new Set<Locale>(['ar'])

/** True when the locale is written right to left. */
export function isRTL(l: Locale = current): boolean {
  return RTL_LOCALES.has(l)
}

/**
 * Mirrors the document to the locale's writing direction, and keeps `lang`
 * truthful for screen readers and font fallback. Direction is decided here and
 * nowhere else: flex rows, logical padding, and scrollbars all follow `dir`
 * from the cascade, so no component needs a per-locale branch.
 */
export function applyDocumentDir(l: Locale = current): void {
  if (typeof document === 'undefined') return
  const el = document.documentElement
  el.lang = l
  el.dir = isRTL(l) ? 'rtl' : 'ltr'
}

let current: Locale = detectLocale()
const listeners = new Set<() => void>()

// First paint already needs the right direction, so apply on load, not on the
// first change.
applyDocumentDir(current)

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
  applyDocumentDir(l)
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
