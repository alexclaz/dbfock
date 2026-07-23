import type { Config } from 'tailwindcss'

export default <Partial<Config>>{
	 darkMode: 'class',
	content: ['./app/**/*.{vue,js,ts}'],
  theme: {
    extend: {
      fontFamily: {
        sans: ['var(--app-font)'],
        mono: ['var(--code-font)'],
      },
      colors: {
        canvas: 'rgb(var(--canvas) / <alpha-value>)', panel: 'rgb(var(--panel) / <alpha-value>)', line: 'rgb(var(--line) / <alpha-value>)', ink: 'rgb(var(--ink) / <alpha-value>)', muted: 'rgb(var(--muted) / <alpha-value>)', accent: 'rgb(var(--accent) / <alpha-value>)',
        'json-key': 'rgb(var(--syntax-key) / <alpha-value>)', 'json-string': 'rgb(var(--syntax-string) / <alpha-value>)', 'json-number': 'rgb(var(--syntax-number) / <alpha-value>)', 'json-boolean': 'rgb(var(--syntax-boolean) / <alpha-value>)', 'json-null': 'rgb(var(--syntax-null) / <alpha-value>)',
      },
      boxShadow: { panel: '0 12px 35px rgb(15 23 42 / 0.08)' },
    },
  },
}
