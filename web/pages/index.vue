<template>
  <div>
    <h1 class="text-4xl font-bold text-sky-800 mb-8 text-center">Cruzeiros Disponíveis</h1>

    <!-- Secção de Interesse em Promoções -->
    <div class="max-w-2xl mx-auto bg-white p-6 rounded-xl shadow-md mb-10 text-center">
        <h3 class="text-xl font-bold text-slate-700 mb-3">Notificações de Promoções</h3>
        <p class="text-slate-600 mb-4">
            <span v-if="isInterestedInPromo">Você está inscrito para receber as nossas melhores ofertas em tempo real.</span>
            <span v-else>Gostaria de receber notificações sobre promoções exclusivas?</span>
        </p>
        <button @click="handleToggleInterest" :disabled="interestPending"
                :class="isInterestedInPromo ? 'bg-amber-500 hover:bg-amber-600' : 'bg-sky-500 hover:bg-sky-600'"
                class="text-white font-semibold py-2 px-5 rounded-lg transition-colors disabled:opacity-50">
            <span v-if="interestPending">A atualizar...</span>
            <span v-else-if="isInterestedInPromo">Cancelar Inscrição</span>
            <span v-else>Inscrever-se</span>
        </button>
        <p v-if="interestError" class="text-red-500 text-sm mt-2">{{ interestError }}</p>
    </div>


    <div v-if="pending" class="text-center text-slate-600">
      <p class="text-lg">A carregar cruzeiros...</p>
      <div class="mt-2 w-16 h-16 border-4 border-dashed rounded-full animate-spin border-sky-600 mx-auto"></div>
    </div>
    <div v-else-if="error" class="text-center text-red-600 bg-red-100 p-4 rounded-lg">
      <p class="font-semibold">Erro ao carregar cruzeiros:</p>
      <p>{{ error.message || error }}</p>
    </div>
    <div v-else-if="cruzeiros && cruzeiros.length" class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-8">
      <CruiseCard v-for="cruzeiro in cruzeiros" :key="cruzeiro.id" :cruzeiro="cruzeiro" />
    </div>
    <div v-else class="text-center text-slate-600">
      Nenhum cruzeiro disponível no momento.
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue';
import CruiseCard from '~/components/CruiseCard.vue';
import { useSseStore } from '~/stores/sseStore';

interface Cruzeiro {
  id: number;
  nome: string;
  empresa: string;
  itinerario: string[];
  portoEmbarque: string;
  portoDesembarque: string;
  dataEmbarque: string;
  dataDesembarque: string;
  cabinesDisponiveis: number;
  valorCabine: number;
  imagemURL?: string;
  descricaoDetalhada?: string;
}

const API_BASE_URL = 'http://localhost:8080';

const { data: cruzeiros, pending, error } = await useAsyncData<Cruzeiro[]>(
    'itineraries',
    () => $fetch(`${API_BASE_URL}/itineraries`)
);

// Lógica de Interesse em Promoções
const sseStore = useSseStore();
const isInterestedInPromo = computed(() => sseStore.isInterestedInPromo);
const interestPending = ref(false);
const interestError = ref<string | null>(null);

async function handleToggleInterest() {
    interestPending.value = true;
    interestError.value = null;
    try {
        await sseStore.togglePromotionInterest();
    } catch (err: any) {
        interestError.value = "Falha ao atualizar a sua preferência. Tente novamente.";
    } finally {
        interestPending.value = false;
    }
}


useHead({
  title: 'Cruzeiros Disponíveis - Reserva de Cruzeiros',
})
</script>
