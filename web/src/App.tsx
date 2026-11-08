import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import { Layout } from './components/Layout';
import { LoginPage } from './pages/Login';
import { CustomerDashboard } from './pages/customer/Dashboard';
import { CustomerCaseDetail } from './pages/customer/CaseDetail';
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
      { path: 'customer/cases/:id', element: <CustomerCaseDetail /> },
      { path: 'merchant/cases', element: <MerchantDashboard /> },
      { path: 'merchant/cases/:id', element: <MerchantCaseDetail /> },
      { path: 'demo', element: <DemoPanel /> },
    ],
  },
]);

export default function Root() {
  return <RouterProvider router={router} />;
}
