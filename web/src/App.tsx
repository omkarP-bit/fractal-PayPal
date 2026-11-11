import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { Toaster } from 'sonner';
import { Layout } from './components/Layout';
import { LoginPage } from './pages/Login';
import { CustomerDashboard } from './pages/customer/Dashboard';
import { CustomerCaseDetail } from './pages/customer/CaseDetail';
import { NewCase } from './pages/customer/NewCase';
import { MerchantDashboard } from './pages/merchant/Dashboard';
import { MerchantCaseDetail } from './pages/merchant/CaseDetail';
import { DemoPanel } from './pages/demo/DemoPanel';

const router = createBrowserRouter([
  {
    path: '/',
    element: <Layout />,
    children: [
      { index: true, element: <CustomerDashboard /> },
      { path: 'login', element: <LoginPage /> },
      { path: 'customer/cases', element: <CustomerDashboard /> },
      { path: 'customer/cases/new', element: <NewCase /> },
      { path: 'customer/cases/:id', element: <CustomerCaseDetail /> },
      { path: 'merchant/cases', element: <MerchantDashboard /> },
      { path: 'merchant/cases/:id', element: <MerchantCaseDetail /> },
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
              'rounded-lg border border-line bg-surface text-ink shadow-pop font-sans text-body',
            title: 'text-body font-semibold text-ink',
            description: 'text-meta text-muted',
            actionButton: 'rounded-pill bg-blue text-white text-meta font-semibold',
            cancelButton: 'rounded-pill bg-surface-muted text-ink text-meta font-semibold',
            closeButton: 'border-line bg-surface text-muted',
          },
        }}
      />
    </>
  );
}
