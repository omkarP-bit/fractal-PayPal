import { ReactNode } from 'react';

interface CardProps {
  children: ReactNode;
  className?: string;
  header?: ReactNode;
  footer?: ReactNode;
}

export function Card({ children, className, header, footer }: CardProps) {
  return (
    <div className={`bg-white border border-gray-200 rounded-lg shadow-sm ${className}`}>
      {header && <div className="px-6 py-4 border-b border-gray-200">{header}</div>}
      <div className="p-6">{children}</div>
      {footer && <div className="px-6 py-4 border-t border-gray-200">{footer}</div>}
    </div>
  );
}

interface CardHeaderProps {
  title: string;
  description?: string;
  icon?: React.ReactNode;
}

export function CardHeader({ title, description, icon }: CardHeaderProps) {
  return (
    <div className="flex items-center gap-3 mb-4">
      {icon && <div className="w-8 h-8 rounded-lg bg-gray-100 flex items-center justify-center text-gray-600">{icon}</div>}
      <div>
        <h3 className="font-semibold text-gray-900">{title}</h3>
        {description && <p className="text-sm text-gray-500">{description}</p>}
      </div>
    </div>
  );
}

export interface CardBodyProps {
  children: ReactNode;
}

export function CardBody({ children }: CardBodyProps) {
  return <div className="p-6">{children}</div>;
}

export interface CardFooterProps {
  children: ReactNode;
}

export function CardFooter({ children }: CardBodyProps) {
  return <div className="px-6 py-4 border-t border-gray-200">{children}</div>;
}
