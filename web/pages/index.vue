<template>
  <div>
    <h1 class="text-4xl font-bold text-sky-800 mb-8 text-center">Cruzeiros Disponíveis</h1>
    <div v-if="pending" class="text-center text-slate-600">
      Carregando cruzeiros...
    </div>
    <div v-else-if="error" class="text-center text-red-600 bg-red-100 p-4 rounded-lg">
      Erro ao carregar cruzeiros: {{ error.message || error }}
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
import CruiseCard from '~/components/CruiseCard.vue';

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
    'cruzeiros',
    () => $fetch(`${API_BASE_URL}/cruzeiros`)
);

useHead({
  title: 'Cruzeiros Disponíveis - Reserva de Cruzeiros',
})
</script>