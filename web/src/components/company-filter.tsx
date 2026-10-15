"use client";

import * as React from "react";

import { api } from "@/lib/api";

interface CompanyFilterValue {
  companyId: number | null;
  setCompanyId: (companyId: number | null) => void;
}

const CompanyFilterContext = React.createContext<CompanyFilterValue>({
  companyId: null,
  setCompanyId: () => {},
});

export function CompanyFilterProvider({ children }: { children: React.ReactNode }) {
  const [companyId, setCompanyId] = React.useState<number | null>(null);
  const value = React.useMemo(() => ({ companyId, setCompanyId }), [companyId]);
  return <CompanyFilterContext.Provider value={value}>{children}</CompanyFilterContext.Provider>;
}

export function useCompanyFilter() {
  return React.useContext(CompanyFilterContext);
}

export function CompanyFilterControl() {
  const { companyId, setCompanyId } = useCompanyFilter();
  const [companies, setCompanies] = React.useState<{ id: number; name: string }[]>([]);

  React.useEffect(() => {
    let alive = true;
    api
      .companies()
      .then((rows) => {
        if (alive) setCompanies(rows.map((company) => ({ id: company.id, name: company.name })));
      })
      .catch(() => {
        if (alive) setCompanies([]);
      });
    return () => {
      alive = false;
    };
  }, []);

  return (
    <label className="group-data-[collapsible=icon]:hidden flex flex-col gap-1 px-2 pb-2 text-xs">
      <span className="text-sidebar-foreground/70">Company</span>
      <select
        aria-label="Company"
        className="border-sidebar-border bg-background h-8 w-full rounded-md border px-2 text-sm"
        value={companyId ?? ""}
        onChange={(event) => setCompanyId(event.target.value ? Number(event.target.value) : null)}
      >
        <option value="">All companies</option>
        {companies.map((company) => (
          <option key={company.id} value={company.id}>
            {company.name}
          </option>
        ))}
      </select>
    </label>
  );
}
