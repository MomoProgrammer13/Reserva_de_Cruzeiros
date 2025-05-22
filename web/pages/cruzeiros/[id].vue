<template>
  <div class="max-w-4xl mx-auto">
    <div v-if="pending" class="text-center text-slate-600 py-10">Carregando detalhes do cruzeiro...</div>
    <div v-else-if="error || !cruzeiro" class="text-center text-red-600 bg-red-100 p-6 rounded-lg shadow">
      <h2 class="text-2xl font-semibold mb-2">Erro ao carregar cruzeiro</h2>
      <p>{{ error?.message || 'Cruzeiro não encontrado.' }}</p>
      <NuxtLink to="/" class="mt-4 inline-block bg-sky-600 hover:bg-sky-700 text-white font-semibold py-2 px-4 rounded-lg transition-colors">
        Voltar para a lista
      </NuxtLink>
    </div>
    <div v-else class="bg-white shadow-xl rounded-xl overflow-hidden">
      <img :src="cruzeiro.imagemURL || 'https://placehold.co/1200x600/CCCCCC/FFFFFF?text=Cruzeiro'"
           :alt="'Imagem do cruzeiro ' + cruzeiro.nome"
           class="w-full h-96 object-cover">
      <div class="p-8 md:p-10">
        <h1 class="text-4xl font-bold text-sky-800 mb-4">{{ cruzeiro.nome }}</h1>
        <p class="text-lg text-slate-700 mb-6">{{ cruzeiro.descricaoDetalhada || 'Descrição detalhada não disponível.' }}</p>

        <div class="grid grid-cols-1 md:grid-cols-2 gap-6 mb-8">
          <div>
            <h3 class="text-xl font-semibold text-sky-700 mb-2">Detalhes da Viagem</h3>
            <ul class="space-y-1 text-slate-600">
              <li><strong>Empresa:</strong> {{ cruzeiro.empresa }}</li>
              <li><strong>Porto de Embarque:</strong> {{ cruzeiro.portoEmbarque }}</li>
              <li><strong>Porto de Desembarque:</strong> {{ cruzeiro.portoDesembarque }}</li>
              <li><strong>Data de Embarque:</strong> {{ formatDate(cruzeiro.dataEmbarque) }}</li>
              <li><strong>Data de Desembarque:</strong> {{ formatDate(cruzeiro.dataDesembarque) }}</li>
              <li><strong>Cabines Disponíveis:</strong> {{ cruzeiro.cabinesDisponiveis }}</li>
            </ul>
          </div>
          <div>
            <h3 class="text-xl font-semibold text-sky-700 mb-2">Itinerário</h3>
            <ul class="list-disc list-inside space-y-1 text-slate-600">
              <li v-for="(local, index) in cruzeiro.itinerario" :key="index">{{ local }}</li>
            </ul>
          </div>
        </div>

        <div class="text-center md:text-left">
          <p class="text-3xl font-bold text-emerald-600 mb-6">Valor por Cabine: R$ {{ cruzeiro.valorCabine.toFixed(2).replace('.', ',') }}</p>
          <NuxtLink :to="`/reservar/${cruzeiro.id}`"
                    class="inline-block bg-emerald-500 hover:bg-emerald-600 text-white font-bold py-3 px-8 rounded-lg text-lg transition-colors shadow-md hover:shadow-lg">
            Reservar Agora
          </NuxtLink>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
interface CruzeiroDetalhado {
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

const route = useRoute();
const cruzeiroId = computed(() => route.params.id as string);

const API_BASE_URL = 'http://localhost:8080';

const { data: cruzeiro, pending, error } = await useAsyncData<CruzeiroDetalhado>(
    `cruzeiro-${cruzeiroId.value}`,
    () => $fetch(`${API_BASE_URL}/cruzeiros/${cruzeiroId.value}`),
    { watch: [cruzeiroId] } // Observa mudanças no ID do cruzeiro para recarregar
);

function formatDate(dateString: string) {
  if (!dateString) return 'Data não informada';
  const [year, month, day] = dateString.split('-');
  return `${day}/${month}/${year}`;
}

useHead( cruzeiro.value ? {
  title: `${cruzeiro.value.nome} - Detalhes do Cruzeiro`,
  meta: [
    { name: 'description', content: cruzeiro.value.descricaoDetalhada || `Detalhes sobre o cruzeiro ${cruzeiro.value.nome}` }
  ]
} : {
  title: 'Detalhes do Cruzeiro'
});
</script>