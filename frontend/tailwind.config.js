/** @type {import('tailwindcss').Config} */
export default {
  content: ['./index.html', './src/**/*.{vue,js,ts,jsx,tsx}'],
  darkMode: 'class',
  theme: {
    extend: {
      colors: {
        primary: {
          50: '#eff6ff',
          100: '#dbeafe',
          200: '#bfdbfe',
          300: '#93c5fd',
          400: '#60a5fa',
          500: '#3b82f6',
          600: '#2563eb',
          700: '#1d4ed8',
          800: '#1e40af',
          900: '#1e3a8a',
          950: '#172554'
        },
        signal: {
          50: '#fbf6f0',
          100: '#f4e7d4',
          200: '#e8cba8',
          300: '#d9a671',
          400: '#cd8648',
          500: '#c26d2d',
          600: '#a85424',
          700: '#8a4020',
          800: '#71361f',
          900: '#5d2e1d'
        },
        alarm: {
          50: '#fff1f2',
          100: '#ffe4e6',
          200: '#fecdd3',
          300: '#fda4af',
          400: '#fb7185',
          500: '#e11d48',
          600: '#be123c',
          700: '#9f1239',
          800: '#881337',
          900: '#4c0519'
        },
        accent: {
          50: '#f7f6f3',
          100: '#eceae4',
          200: '#d9d5cb',
          300: '#bfb8aa',
          400: '#a09686',
          500: '#867c6d',
          600: '#6d6458',
          700: '#585247',
          800: '#3c3832',
          900: '#24221e',
          950: '#141311'
        },
        dark: {
          50: '#f5f6f7',
          100: '#e6e9ec',
          200: '#c9d0d6',
          300: '#a3aeb8',
          400: '#748492',
          500: '#586978',
          600: '#455462',
          700: '#384552',
          800: '#1c2430',
          900: '#121820',
          950: '#0b0f14'
        }
      },
      fontFamily: {
        sans: [
          '"IBM Plex Sans"',
          'Segoe UI',
          'system-ui',
          '-apple-system',
          'BlinkMacSystemFont',
          'PingFang SC',
          'Microsoft YaHei UI',
          'sans-serif'
        ],
        display: [
          'Space Grotesk',
          'Sora',
          'Outfit',
          '"IBM Plex Sans"',
          'Segoe UI',
          'system-ui',
          'PingFang SC',
          'Microsoft YaHei UI',
          'sans-serif'
        ],
        mono: [
          '"JetBrains Mono"',
          'ui-monospace',
          'SFMono-Regular',
          'Menlo',
          'Monaco',
          'Consolas',
          'monospace'
        ]
      },
      boxShadow: {
        glass: '0 12px 40px rgba(17, 20, 24, 0.08)',
        'glass-sm': '0 6px 18px rgba(17, 20, 24, 0.06)',
        glow: '0 0 0 4px rgba(79, 70, 229, 0.14)',
        'glow-lg': '0 0 0 6px rgba(15, 159, 148, 0.16)',
        card: '0 1px 0 rgba(17,20,24,0.04), 0 14px 34px -22px rgba(17,20,24,0.28)',
        'card-hover': '0 1px 0 rgba(17,20,24,0.05), 0 24px 42px -22px rgba(17,20,24,0.34)',
        'inner-glow': 'inset 0 1px 0 rgba(255,255,255,0.55)',
        desk: '0 0 0 1px rgba(17,20,24,0.05), 0 18px 36px -24px rgba(17,20,24,0.22)'
      },
      backgroundImage: {
        'gradient-radial': 'radial-gradient(var(--tw-gradient-stops))',
        'gradient-primary': 'linear-gradient(135deg, #3b82f6 0%, #1d4ed8 100%)',
        'gradient-dark': 'linear-gradient(160deg, #161d27 0%, #0b0f14 100%)',
        'gradient-glass':
          'linear-gradient(135deg, rgba(255,255,255,0.14) 0%, rgba(255,255,255,0.04) 100%)',
        'gradient-signal': 'linear-gradient(90deg, #0f9f94 0%, #c26d2d 100%)',
        'mesh-gradient':
          'radial-gradient(at 8% 0%, rgba(15,159,148,0.10) 0px, transparent 42%), radial-gradient(at 96% 4%, rgba(194,109,45,0.08) 0px, transparent 40%), radial-gradient(at 50% 100%, rgba(17,20,24,0.04) 0px, transparent 48%)',
        'desk-grid':
          'linear-gradient(rgba(17,20,24,0.035) 1px, transparent 1px), linear-gradient(90deg, rgba(17,20,24,0.035) 1px, transparent 1px)'
      },
      backgroundSize: {
        'desk-grid': '32px 32px'
      },
      animation: {
        'fade-in': 'fadeIn 0.3s ease-out',
        'slide-up': 'slideUp 0.3s ease-out',
        'slide-down': 'slideDown 0.3s ease-out',
        'slide-in-right': 'slideInRight 0.3s ease-out',
        'scale-in': 'scaleIn 0.2s ease-out',
        'pulse-slow': 'pulse 3s cubic-bezier(0.4, 0, 0.6, 1) infinite',
        shimmer: 'shimmer 2s linear infinite',
        glow: 'glow 2s ease-in-out infinite alternate'
      },
      keyframes: {
        fadeIn: {
          '0%': { opacity: '0' },
          '100%': { opacity: '1' }
        },
        slideUp: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideDown: {
          '0%': { opacity: '0', transform: 'translateY(-10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' }
        },
        slideInRight: {
          '0%': { opacity: '0', transform: 'translateX(20px)' },
          '100%': { opacity: '1', transform: 'translateX(0)' }
        },
        scaleIn: {
          '0%': { opacity: '0', transform: 'scale(0.95)' },
          '100%': { opacity: '1', transform: 'scale(1)' }
        },
        shimmer: {
          '0%': { backgroundPosition: '-200% 0' },
          '100%': { backgroundPosition: '200% 0' }
        },
        glow: {
          '0%': { boxShadow: '0 0 0 4px rgba(15, 159, 148, 0.10)' },
          '100%': { boxShadow: '0 0 0 6px rgba(15, 159, 148, 0.18)' }
        }
      },
      backdropBlur: {
        xs: '2px'
      },
      borderRadius: {
        '4xl': '2rem'
      }
    }
  },
  plugins: []
}





