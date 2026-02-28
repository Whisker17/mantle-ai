import nextra from 'nextra'

const withNextra = nextra({
  theme: 'nextra-theme-docs',
  themeConfig: './theme.config.jsx'
})

const repoName = process.env.GITHUB_REPOSITORY?.split('/')[1] ?? ''
const isProduction = process.env.NODE_ENV === 'production'
const basePath = isProduction && repoName ? `/${repoName}` : ''

export default withNextra({
  reactStrictMode: true,
  output: 'export',
  trailingSlash: true,
  images: {
    unoptimized: true
  },
  basePath,
  assetPrefix: basePath || undefined
})
