// https://nuxt.com/docs/api/configuration/nuxt-config
export default defineNuxtConfig({
  devtools: { enabled: true },
  modules: [
    '@nuxtjs/tailwindcss' // Adiciona o módulo Tailwind CSS
  ],
  // Configuração para chamadas de API, se necessário (ex: proxy ou baseURL)
  // runtimeConfig: {
  //   public: {
  //     apiBase: 'http://localhost:8080' // URL do seu backend Go
  //   }
  // },
  app: {
    head: {
      title: 'Reserva de Cruzeiros',
      meta: [
        { charset: 'utf-f8' },
        { name: 'viewport', content: 'width=device-width, initial-scale=1' }
      ],
      link: [
        { rel: 'icon', type: 'image/x-icon', href: '/favicon.ico' }
      ]
    }
  }
})