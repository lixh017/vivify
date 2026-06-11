import { redirect } from 'next/navigation'

// Root path is a vanity entry — bounce to the dashboard. The
// dashboard itself is protected by the console middleware, so
// the redirect target is what guards the user.
export default function HomePage() {
  redirect('/dashboard')
}
