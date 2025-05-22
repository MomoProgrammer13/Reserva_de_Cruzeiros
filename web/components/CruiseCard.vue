<template>
  <div class="bg-white rounded-xl shadow-lg overflow-hidden transition-all hover:shadow-2xl flex flex-col">
    <img :src="cruzeiro.imagemURL || 'https://placehold.co/600x400/CCCCCC/FFFFFF?text=Cruzeiro'"
         :alt="'Imagem do cruzeiro ' + cruzeiro.nome"
         class="w-full h-48 object-cover">
    <div class="p-6 flex flex-col flex-grow">
      <h3 class="text-xl font-semibold text-sky-700 mb-2">{{ cruzeiro.nome }}</h3>
      <p class="text-sm text-slate-600 mb-1"><span class="font-medium">Empresa:</span> {{ cruzeiro.empresa }}</p>
      <p class="text-sm text-slate-600 mb-1"><span class="font-medium">Porto de Embarque:</span> {{ cruzeiro.portoEmbarque }}</p>
      <p class="text-sm text-slate-600 mb-1"><span class="font-medium">Data de Embarque:</span> {{ formatDate(cruzeiro.dataEmbarque) }}</p>
      <p class="text-lg font-bold text-emerald-600 mt-2 mb-4">R$ {{ cruzeiro.valorCabine.toFixed(2).replace('.', ',') }}</p>
      <div class="mt-auto">
        <NuxtLink :to="`/cruzeiros/${cruzeiro.id}`"
                  class="block w-full text-center bg-sky-600 hover:bg-sky-700 text-white font-semibold py-2 px-4 rounded-lg transition-colors">
          Detalhes
        </NuxtLink>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
interface CruzeiroBasico {
  id: number;
  nome: string;
  empresa: string;
  portoEmbarque: string;
  dataEmbarque: string;
  valorCabine: number;
  imagemURL?: string;
}

defineProps<{
  cruzeiro: CruzeiroBasico;
}>();

function formatDate(dateString: string) {
  if (!dateString) return 'Data não informada';
  const [year, month, day] = dateString.split('-');
  return `${day}/${month}/${year}`;
}
</script>