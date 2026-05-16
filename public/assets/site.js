const form = document.querySelector("#signup-form");
const message = document.querySelector("#form-message");
const authForm = document.querySelector("#auth-form");
const authMessage = document.querySelector("#auth-message");
const tabs = document.querySelectorAll(".tab");
const companyField = document.querySelector(".company-field");
const accountState = document.querySelector(".account-state");
const accountName = document.querySelector("#account-name");
const accountEmail = document.querySelector("#account-email");
const logoutButton = document.querySelector("#logout-button");
const newsletterForm = document.querySelector(".newsletter-form");
const newsletterMessage = document.querySelector(".newsletter-message");
const statusPageForm = document.querySelector("#status-page-form");
const statusPageMessage = document.querySelector("#status-page-message");
const statusPagesList = document.querySelector("#status-pages-list");
const domainForm = document.querySelector("#domain-form");
const domainMessage = document.querySelector("#domain-message");
const domainsList = document.querySelector("#domains-list");
const checkoutDialog = document.querySelector("#checkout-dialog");
const checkoutContainer = document.querySelector("#checkout-container");
let authMode = "register";
let currentUser = null;
let stripePublishableKey = "";
let embeddedCheckout = null;

const checkoutState = new URLSearchParams(window.location.search).get("checkout");
if (checkoutState === "success") {
  messageForPage("Payment received. We will email you with setup next steps.");
}
if (checkoutState === "cancelled") {
  messageForPage("Checkout cancelled. You can restart whenever you are ready.");
}

function messageForPage(text) {
  const signup = document.querySelector("#signup");
  if (!signup) return;
  signup.scrollIntoView({ block: "center" });
  const p = document.querySelector("#form-message");
  if (p) p.textContent = text;
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  message.textContent = "Creating checkout...";
  const data = Object.fromEntries(new FormData(form).entries());
  if (currentUser) {
    data.email ||= currentUser.email;
    data.company ||= currentUser.company;
  }

  try {
    await openEmbeddedCheckout(data);
  } catch (err) {
    message.textContent = err.message;
  }
});

async function openEmbeddedCheckout(data) {
  if (!stripePublishableKey) await loadConfig();
  if (!stripePublishableKey || !window.Stripe || !checkoutDialog || !checkoutContainer) {
    const res = await fetch("/checkout/create", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
    const body = await res.json();
    if (!res.ok) {
      throw new Error(body.message || body.error || "Checkout is unavailable");
    }
    window.location.href = body.url;
    return;
  }

  const res = await fetch("/checkout/create-embedded", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
  const body = await res.json();
  if (!res.ok) throw new Error(body.message || body.error || "Checkout is unavailable");
  checkoutContainer.innerHTML = "";
  if (embeddedCheckout) {
    embeddedCheckout.destroy();
    embeddedCheckout = null;
  }
  checkoutDialog.showModal();
  const stripe = window.Stripe(stripePublishableKey);
  embeddedCheckout = await stripe.initEmbeddedCheckout({
    clientSecret: body.client_secret,
  });
  embeddedCheckout.mount("#checkout-container");
  message.textContent = "Checkout opened.";
}

checkoutDialog?.addEventListener("close", () => {
  if (embeddedCheckout) {
    embeddedCheckout.destroy();
    embeddedCheckout = null;
  }
  if (checkoutContainer) checkoutContainer.innerHTML = "";
});

tabs.forEach((tab) => {
  tab.addEventListener("click", () => {
    authMode = tab.dataset.mode;
    tabs.forEach((item) => item.classList.toggle("active", item === tab));
    companyField.hidden = authMode === "login";
    companyField.querySelector("input").required = authMode === "register";
    authForm.querySelector("button[type='submit']").textContent = authMode === "login" ? "Login" : "Create account";
    authMessage.textContent = "";
  });
});

authForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  authMessage.textContent = authMode === "login" ? "Logging in..." : "Creating account...";
  const data = Object.fromEntries(new FormData(authForm).entries());
  const endpoint = authMode === "login" ? "/api/login" : "/api/register";
  try {
    const res = await fetch(endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
    const body = await res.json();
    if (!res.ok) throw new Error(body.error || "Account request failed");
    setUser(body.user);
    localStorage.setItem("statuspageHasSession", "true");
    authMessage.textContent = authMode === "login" ? "Logged in." : "Account created.";
    if (form) {
      form.email.value = body.user.email;
      form.company.value = body.user.company;
    }
    loadStatusPages();
  } catch (err) {
    authMessage.textContent = err.message;
  }
});

logoutButton?.addEventListener("click", async () => {
  await fetch("/api/logout", { method: "POST" });
  localStorage.removeItem("statuspageHasSession");
  setUser(null);
  if (statusPagesList) statusPagesList.innerHTML = "";
  if (domainsList) domainsList.innerHTML = "";
});

newsletterForm?.addEventListener("submit", (event) => {
  event.preventDefault();
  const email = new FormData(newsletterForm).get("email");
  newsletterMessage.textContent = email ? "Thanks. We will send occasional product updates." : "Enter an email to subscribe.";
  newsletterForm.reset();
});

async function loadCurrentUser() {
  if (localStorage.getItem("statuspageHasSession") !== "true") return;
  try {
    const res = await fetch("/api/me");
    if (!res.ok) {
      localStorage.removeItem("statuspageHasSession");
      return;
    }
    const body = await res.json();
    setUser(body.user);
    loadStatusPages();
  } catch {
    localStorage.removeItem("statuspageHasSession");
    setUser(null);
  }
}

function setUser(user) {
  currentUser = user;
  if (accountState && authForm) {
    accountState.hidden = !user;
    authForm.hidden = !!user;
  }
  if (user) {
    accountName.textContent = user.company;
    accountEmail.textContent = user.email;
    if (form) {
      form.email.value = user.email;
      form.company.value = user.company;
    }
  }
}

async function loadConfig() {
  try {
    const res = await fetch("/api/config");
    if (!res.ok) return;
    const body = await res.json();
    stripePublishableKey = body.stripe_publishable_key || "";
  } catch {}
}

statusPageForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  statusPageMessage.textContent = "Creating status page...";
  const data = Object.fromEntries(new FormData(statusPageForm).entries());
  try {
    const res = await fetch("/api/v1/status-pages", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
    const body = await res.json();
    if (!res.ok && body.error !== "slug_already_taken") throw new Error(body.error || "Create failed");
    statusPageMessage.textContent = body.error === "slug_already_taken" ? "Status page already exists." : `Created ${body.url}.`;
    await loadStatusPages();
  } catch (err) {
    statusPageMessage.textContent = err.message;
  }
});

domainForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  domainMessage.textContent = "Provisioning subdomain...";
  const data = Object.fromEntries(new FormData(domainForm).entries());
  const slug = data.slug;
  try {
    const res = await fetch(`/api/v1/status-pages/${encodeURIComponent(slug)}/domains`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ subdomain: data.subdomain, type: "statuspage_subdomain" }),
    });
    const body = await res.json();
    if (!res.ok) throw new Error(body.last_error || body.error || "Domain failed");
    domainMessage.textContent = `${body.hostname}: ${body.status}`;
    await loadDomains(slug);
  } catch (err) {
    domainMessage.textContent = err.message;
  }
});

async function loadStatusPages() {
  if (!statusPagesList || !currentUser) return;
  const res = await fetch("/api/v1/status-pages");
  if (!res.ok) return;
  const body = await res.json();
  const pages = body.status_pages || [];
  statusPagesList.innerHTML = pages.map((page) => `<p><a href="${page.url}">${page.name}</a><span>${page.slug}</span></p>`).join("") || "<p>No status pages yet.</p>";
  if (pages[0]?.slug) {
    if (domainForm?.slug) domainForm.slug.value = pages[0].slug;
    loadDomains(pages[0].slug);
  }
}

async function loadDomains(slug) {
  if (!domainsList || !slug) return;
  const res = await fetch(`/api/v1/status-pages/${encodeURIComponent(slug)}/domains`);
  if (!res.ok) return;
  const body = await res.json();
  const domains = body.domains || [];
  domainsList.innerHTML = domains.map((domain) => `<p><a href="https://${domain.hostname}">${domain.hostname}</a><span>${domain.status}</span></p>`).join("") || "<p>No domains yet.</p>";
}

loadConfig();
loadCurrentUser();
