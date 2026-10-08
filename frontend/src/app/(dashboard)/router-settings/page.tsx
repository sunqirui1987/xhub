import { redirect } from "next/navigation";

/** The old address. Load balancing and fallbacks now live on the template library. */
export default function RouterSettingsPage() {
  redirect("/route-templates");
}
