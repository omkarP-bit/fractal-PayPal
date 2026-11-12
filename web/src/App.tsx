import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { Toaster } from 'sonner';
import { Layout } from './components/Layout';
import { LoginPage } from './pages/Login';
import { CustomerDashboard } from './pages/customer/Dashboard';
import { CustomerCaseDetail } from './pages/customer/CaseDetail';
import { NewCase } from './pages/customer/NewCase';
import { MerchantDashboard } from './pages/merchant/Dashboard';
import { MerchantCaseDetail } from './pages/merchant/CaseDetail';
import { MerchantOnboardingPage, MerchantSignupPage } from './pages/merchant/Onboarding';
import { DemoPanel } from './pages/demo/DemoPanel';

const router = createBrowserRouter([
  {
    path: '/',
    element: <Layout />,
    children: [
      { index: true, element: <CustomerDashboard /> },
      { path: 'login', element: <LoginPage /> },
      { path: 'signup', element: <MerchantSignupPage /> },
      { path: 'customer/cases', element: <CustomerDashboard /> },
      { path: 'customer/cases/new', element: <NewCase /> },
      { path: 'customer/cases/:id', element: <CustomerCaseDetail /> },
      { path: 'merchant/cases', element: <MerchantDashboard /> },
      { path: 'merchant/cases/:id', element: <MerchantCaseDetail /> },
      { path: 'merchant/onboarding', element: <MerchantOnboardingPage /> },
      { path: 'demo', element: <DemoPanel /> },
    ],
  },
]);

export default function Root() {
  return (
    <>
      <RouterProvider router={router} />
      <Toaster
        position="bottom-right"
        offset={24}
        mobileOffset={{ bottom: 80, right: 16, left: 16 }}
        gap={10}
        toastOptions={{
          classNames: {
            toast:
              'rounded border-2 border-line bg-surface text-ink shadow-hard-lg font-sans text-body',
            title: 'text-body font-extrabold text-ink',
            description: 'text-meta text-muted',
            actionButton: 'rounded border-2 border-line bg-blue text-[color:var(--text-on-blue)] text-meta font-extrabold',
            cancelButton: 'rounded border-2 border-line bg-surface-alt text-ink text-meta font-extrabold',
            closeButton: 'border-line bg-surface text-ink',
          },
        }}
      />
    </>
  );
}
