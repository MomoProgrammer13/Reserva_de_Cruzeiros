<template>
  <div class="min-h-screen flex flex-col">
    <header class="bg-sky-700 text-white shadow-md">
      <nav class="container mx-auto px-4 py-4 flex justify-between items-center">
        <NuxtLink to="/" class="text-2xl font-bold hover:text-sky-200 transition-colors">Reserva de Cruzeiros</NuxtLink>
      </nav>
    </header>

    <main class="flex-grow container mx-auto px-4 py-8">
      <slot />
    </main>

    <footer class="bg-slate-800 text-slate-300 text-center p-4">
      <p>&copy; {{ new Date().getFullYear() }} Reserva de Cruzeiros. Todos os direitos reservados.</p>
    </footer>
  </div>
</template>

<script setup lang="ts">
</script>

<style scoped>
/* Estilos específicos do layout */
</style>