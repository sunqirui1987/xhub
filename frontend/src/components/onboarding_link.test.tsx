import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { buildOnboardingUrl } from "./onboarding_link";

describe("buildOnboardingUrl", () => {
  const originalLocation = window.location;

  beforeEach(() => {
    Object.defineProperty(window, "location", {
      value: {
        ...originalLocation,
        origin: "http://localhost:3000",
        pathname: "/playground",
      },
      writable: true,
    });
  });

  afterEach(() => {
    Object.defineProperty(window, "location", {
      value: originalLocation,
      writable: true,
    });
  });

  it("points the invitation link at the console /ui/onboarding route, not the API origin", () => {
    expect(
      buildOnboardingUrl({
        baseUrl: "http://localhost:4000/",
        invitationId: "inv-123",
        hasUserSetupSso: false,
        resetPassword: false,
      }),
    ).toBe("http://localhost:3000/ui/onboarding?invitation_id=inv-123");
  });

  it("preserves a console mount prefix that sits before /ui", () => {
    Object.defineProperty(window, "location", {
      value: {
        ...originalLocation,
        origin: "https://console.example.com",
        pathname: "/litellm/ui/users",
      },
      writable: true,
    });
    expect(
      buildOnboardingUrl({
        baseUrl: "https://api.example.com",
        invitationId: "inv-123",
        hasUserSetupSso: false,
        resetPassword: false,
      }),
    ).toBe("https://console.example.com/litellm/ui/onboarding?invitation_id=inv-123");
  });

  it("appends action=reset_password for the reset-password flow", () => {
    expect(
      buildOnboardingUrl({
        baseUrl: "http://localhost:4000/",
        invitationId: "inv-123",
        hasUserSetupSso: false,
        resetPassword: true,
      }),
    ).toBe("http://localhost:3000/ui/onboarding?invitation_id=inv-123&action=reset_password");
  });

  it("sends SSO users to the console dashboard, not the API origin", () => {
    expect(
      buildOnboardingUrl({
        baseUrl: "http://localhost:4000/",
        invitationId: "inv-123",
        hasUserSetupSso: true,
        resetPassword: false,
      }),
    ).toBe("http://localhost:3000/ui");
  });

  it("builds the console link when the API base is empty", () => {
    expect(
      buildOnboardingUrl({
        baseUrl: "",
        invitationId: "inv-123",
        hasUserSetupSso: false,
        resetPassword: false,
      }),
    ).toBe("http://localhost:3000/ui/onboarding?invitation_id=inv-123");
  });

  it("returns an empty string rather than an invitation_id=undefined link when the id is not ready", () => {
    expect(
      buildOnboardingUrl({
        baseUrl: "http://localhost:4000/",
        invitationId: undefined,
        hasUserSetupSso: false,
        resetPassword: false,
      }),
    ).toBe("");
  });
});
