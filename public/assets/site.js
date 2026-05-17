const form = document.querySelector("#signup-form");
const message = document.querySelector("#form-message");
const billingDialog = document.querySelector("#billing-dialog");
const billingForm = document.querySelector("#billing-form");
const openBillingButton = document.querySelector("#open-billing-button");
const authForm = document.querySelector("#auth-form");
const authMessage = document.querySelector("#auth-message");
const resetForm = document.querySelector("#reset-form");
const resetMessage = document.querySelector("#reset-message");
const tabs = document.querySelectorAll(".tab");
const companyField = document.querySelector(".company-field");
const passwordField = document.querySelector(".password-field");
const domainField = document.querySelector(".domain-field");
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
let appNZLoginURL = "";
let embeddedCheckout = null;
const params = new URLSearchParams(window.location.search);
const resetToken = params.get("reset_token");
const resetEmail = params.get("email") || "";

const checkoutState = params.get("checkout");
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
  showBillingDialog();
});

openBillingButton?.addEventListener("click", showBillingDialog);

billingForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (message) message.textContent = "Creating checkout...";
  const data = checkoutData();
  const selected = new FormData(billingForm).get("plan");
  data.plan = selected || data.plan || "annual";
  try {
    await openEmbeddedCheckout(data);
  } catch (err) {
    if (message) message.textContent = err.message;
  }
});

function showBillingDialog() {
  const data = checkoutData();
  if (!currentUser && !data.email && authForm) {
    authMessage.textContent = "Create an account or login first.";
    authForm.scrollIntoView({ block: "center" });
    return;
  }
  if (billingDialog) {
    if (message) message.textContent = "";
    billingDialog.showModal();
    return;
  }
  openEmbeddedCheckout(data).catch((err) => {
    if (message) message.textContent = err.message;
  });
}

function checkoutData() {
  const data = form ? Object.fromEntries(new FormData(form).entries()) : {};
  const authData = authForm ? Object.fromEntries(new FormData(authForm).entries()) : {};
  data.email ||= currentUser?.email || authData.email || accountEmail?.textContent || "";
  data.company ||= currentUser?.company || authData.company || accountName?.textContent || companyFromEmail(data.email);
  data.domain ||= authData.domain || "";
  data.plan ||= "annual";
  return data;
}

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
  if (message) message.textContent = "Checkout opened.";
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
    const isLogin = authMode === "login";
    const isForgot = authMode === "forgot";
    if (companyField) {
      companyField.hidden = isLogin || isForgot;
      companyField.querySelector("input").required = authMode === "register";
    }
    if (domainField) domainField.hidden = isLogin || isForgot;
    if (passwordField) {
      passwordField.hidden = isForgot;
      passwordField.querySelector("input").required = !isForgot;
    }
    authForm.querySelector("button[type='submit']").textContent = isForgot ? "Send reset link" : isLogin ? "Login" : "Create account and continue";
    authMessage.textContent = "";
  });
});

authForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (authMode === "forgot") {
    await requestPasswordReset();
    return;
  }
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
      if (form.domain && data.domain) form.domain.value = data.domain;
    }
    loadStatusPages();
    if (data.domain || document.body.classList.contains("account-page") === false) {
      showBillingDialog();
    }
  } catch (err) {
    authMessage.textContent = err.message;
  }
});

async function requestPasswordReset() {
  authMessage.textContent = "Sending reset link...";
  const data = Object.fromEntries(new FormData(authForm).entries());
  try {
    const res = await fetch("/api/forgot-password", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email: data.email }),
    });
    const body = await res.json();
    if (!res.ok) throw new Error(body.error || "Could not send reset link");
    authMessage.textContent = "If that app.nz account exists, a reset link has been emailed.";
  } catch (err) {
    authMessage.textContent = err.message;
  }
}

resetForm?.addEventListener("submit", async (event) => {
  event.preventDefault();
  resetMessage.textContent = "Resetting password...";
  const data = Object.fromEntries(new FormData(resetForm).entries());
  data.token = resetToken;
  try {
    const res = await fetch("/api/reset-password", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
    const body = await res.json();
    if (!res.ok) throw new Error(body.error || "Reset failed");
    setUser(body.user);
    localStorage.setItem("statuspageHasSession", "true");
    resetMessage.textContent = "Password reset. You are signed in.";
    loadStatusPages();
  } catch (err) {
    resetMessage.textContent = err.message;
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
    appNZLoginURL = body.appnz_login_url || "";
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
if (resetToken && resetForm && authForm) {
  resetForm.hidden = false;
  authForm.hidden = true;
  if (resetForm.email) resetForm.email.value = resetEmail;
}
loadCurrentUser();

function companyFromEmail(email) {
  return String(email || "").split("@")[0].replace(/[._-]+/g, " ") || "app.nz user";
}
