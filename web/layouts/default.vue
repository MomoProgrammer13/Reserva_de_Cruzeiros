<template>
  <div class="min-h-screen flex flex-col bg-slate-50">
    <header class="bg-sky-700 text-white shadow-md sticky top-0 z-50">
      <nav class="container mx-auto px-4 py-4 flex justify-between items-center">
        <NuxtLink to="/" class="text-2xl font-bold hover:text-sky-200 transition-colors">Reserva de Cruzeiros</NuxtLink>
        <div class="flex items-center gap-6">
            <NuxtLink to="/" class="text-lg hover:text-sky-200 transition-colors">Início</NuxtLink>
            <NuxtLink to="/meus-bilhetes" class="text-lg hover:text-sky-200 transition-colors">Meus Bilhetes</NuxtLink>
        </div>
      </nav>
    </header>

    <main class="flex-grow container mx-auto px-4 py-8">
      <slot />
    </main>

    <!-- Notificação Global de Promoção -->
    <div v-if="sseStore.lastPromotion"
         class="fixed bottom-10 right-10 bg-white border-2 border-sky-500 text-slate-800 px-6 py-4 rounded-lg shadow-2xl z-50 max-w-sm">
        <div class="flex items-start">
            <div class="text-sky-500 mr-4">
                <svg xmlns="http://www.w3.org/2000/svg" class="h-8 w-8" viewBox="0 0 20 20" fill="currentColor">
                    <path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a1 1 0 000 2v3a1 1 0 001 1h1a1 1 0 100-2v-3a1 1 0 00-1-1H9z" clip-rule="evenodd" />
                </svg>
            </div>
            <div>
                <strong class="font-bold block text-lg">{{ sseStore.lastPromotion.nomePromocao }}</strong>
                <span class="block mt-1">{{ sseStore.lastPromotion.descricao }}</span>
            </div>
            <button @click="sseStore.clearPromotion()" class="ml-4 text-slate-400 hover:text-slate-600">&times;</button>
        </div>
    </div>


    <footer class="bg-slate-800 text-slate-300 text-center p-4">
      <p>&copy; {{ new Date().getFullYear() }} Reserva de Cruzeiros. Todos os direitos reservados.</p>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { useTicketStore } from '~/stores/ticketStore';
import { useSseStore } from '~/stores/sseStore';
import { onMounted } from 'vue';

const ticketStore = useTicketStore();
const sseStore = useSseStore();

// Garante que os bilhetes e a conexão SSE sejam iniciados com o app
onMounted(() => {
  ticketStore.loadTicketsFromStorage();
  sseStore.connect();
});
</script>
