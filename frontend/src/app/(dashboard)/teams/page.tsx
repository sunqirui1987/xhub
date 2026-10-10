"use client";

import { QuotaGuide } from "@/components/shared/QuotaGuide";

import Teams from "@/components/Teams";
import useAuthorized from "@/app/(dashboard)/hooks/useAuthorized";

export default function TeamsPage() {
  const { accessToken, userId, userRole, premiumUser } = useAuthorized();
  return <><QuotaGuide scope="team" /><Teams accessToken={accessToken} userID={userId} userRole={userRole} premiumUser={premiumUser ?? false} /></>;
}
