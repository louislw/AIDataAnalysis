import { createRouter, createWebHistory } from 'vue-router'
import { useSession } from './stores/session'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/login', component: () => import('./views/Login.vue') },
    { path: '/', component: () => import('./views/Home.vue'), meta: { auth: true } },
    { path: '/p/:personaId', component: () => import('./views/Chat.vue'), meta: { auth: true } },
    { path: '/admin/datasources', component: () => import('./views/Datasources.vue'), meta: { admin: true } },
    { path: '/admin/datasources/:id', component: () => import('./views/DatasourceDetail.vue'), meta: { admin: true } },
    { path: '/admin/personas', component: () => import('./views/Personas.vue'), meta: { admin: true } },
    { path: '/admin/personas/:id', component: () => import('./views/PersonaDetail.vue'), meta: { admin: true } },
  ],
})

router.beforeEach((to) => {
  const session = useSession()
  if ((to.meta.auth || to.meta.admin) && !session.token) return '/login'
  if (to.meta.admin && session.role !== 'ADMIN') return '/'
  if (to.path === '/login' && session.token) return '/'
})

export default router
