export default {
  logo: <span>Mantle AI Docs</span>,
  project: {
    link: 'https://github.com/mantle/mantle-ai'
  },
  docsRepositoryBase: 'https://github.com/mantle/mantle-ai/tree/main/docs',
  footer: {
    text: 'Mantle AI Documentation'
  },
  useNextSeoProps() {
    return {
      titleTemplate: '%s - Mantle AI Docs'
    }
  }
}
