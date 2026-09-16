import {
    c as b,
    k as Re,
    w as we,
    Q as Ce,
    ag as De,
    U as I,
    V as t,
    X as s,
    W as r,
    u as l,
    a4 as J,
    a6 as x,
    a7 as be,
    af as Le,
    d as ge,
    a1 as Z,
    av as ee,
    L as N
} from "./vendor-BCSkBb3Z.js";
import {
    d as Pe,
    s as Oe
} from "./vue-core-MrmoKyQJ.js";
import {
    c as j,
    b as z,
    h as We,
    f as je
} from "./route-sms-3G-7c3Qf.js";
import {
    d as se
} from "./route-devices-uPYlkLrw.js";
import {
    b as ze,
    c as He,
    a as Me,
    u as qe,
    i as c,
    w as Qe,
    q as Ke,
    e as $e,
    f as Xe,
    z as Ge,
    j as ke
} from "./element-plus-DL-K_RFc.js";
import {
    K as Ye,
    g as Ve,
    v as Je,
    c as he,
    w as Ue,
    x as ve,
    y as fe
} from "./vicons-DR2NoM2D.js";
import "./dayjs-CEZ7Y9on.js";
const H = {
        changePassword(_) {
            return j(async () => (await z.post("/settings/password", _), !0))
        },
        getNotifications() {
            return j(async () => (await z.get("/settings/notifications")).data || {})
        },
        saveNotifications(_) {
            return j(async () => {
                var U, k;
                const g = await z.put("/settings/notifications", _);
                return {
                    applied: (U = g.data) == null ? void 0 : U.applied,
                    warning: (k = g.data) == null ? void 0 : k.warning
                }
            })
        },
        testWebhook(_) {
            return j(async () => (await z.post("/settings/notifications/webhook/test", _)).data)
        },
        testBark(_) {
            return j(async () => (await z.post("/settings/notifications/bark/test", _)).data)
        },
        testEmail(_) {
            return j(async () => (await z.post("/settings/notifications/email/test", _)).data)
        },

    },
    Ze = {
        version: "",
        build_time: "",
        config: "",
        docs: {
            swagger_ui: "",
            openapi_yaml: "",
            openapi_json: ""
        }
    },
    Se = {
        old_password: "",
        new_password: "",
        confirm_password: ""
    },
    es = {
        enabled: !1,
        bot_token: "",
        chat_id: null,
        admin_id: null,
        base_url: "",
        proxy: ""
    },
    ss = {
        enabled: !1,
        app_id: "",
        app_secret: "",
        chat_ids: ""
    },
    ls = {
        enabled: !1,
        app_id: "",
        app_secret: "",
        group_ids: "",
        direct_ids: ""
    },
    ts = {
        enabled: !1,
        use_ssl: !1,
        smtp_host: "",
        smtp_port: null,
        username: "",
        password: "",
        from_address: "",
        to_addresses: ""
    },
    as = {
        enabled: !1,
        token: "",
        topic: "",
        channel: "wechat"
    },
    os = {
        enabled: !1,
        urls: [],
        secret: "",
        timeout_ms: 5e3,
        retry_max: 3,
        text_template: "{{device_label}} {{text}}",
        headers: {}
    },
    rs = new Set(["content-type", "x-vohive-signature"]);

function Ee(_) {
    const g = {};
    if (!_) return g;
    for (const [U, k] of Object.entries(_)) {
        const R = String(U || "").trim();
        !R || rs.has(R.toLowerCase()) || (g[R] = String(k ?? ""))
    }
    return g
}
const ds = {
        enabled: !1,
        urls: [],
        group: "vohive",
        icon: "",
        level: "active"
    },
    ns = Pe("settings", () => {
        const _ = b({
                ...Ze
            }),
            g = b({}),
            U = b({
                ...Se
            }),
            k = b({
                ...es
            }),
            R = b({
                ...ss
            }),
            W = b({
                ...ls
            }),
            w = b({
                ...os
            }),
            h = b({
                ...ds
            }),
            v = b({
                ...ts
            }),
            S = b({
                ...as
            }),
            y = b(!1),
            E = b(!1),
            V = b(!1),
            d = b(!1),
            p = b(!1),
            n = b(!1),
            A = b(!1),
            C = b(null);
        async function te() {
            E.value = !0;
            const u = await H.getNotifications();
            if (u.ok) {
                g.value = u.data || {};
                const m = u.data.telegram || {},
                    F = u.data.feishu || {},
                    D = u.data.qq || {},
                    L = u.data.webhook || {};
                k.value = {
                    enabled: !!m.enabled,
                    bot_token: m.bot_token || "",
                    chat_id: m.chat_id ?? null,
                    admin_id: m.admin_id ?? null,
                    base_url: m.base_url || "",
                    proxy: m.proxy || ""
                }, R.value = {
                    enabled: !!F.enabled,
                    app_id: F.app_id || "",
                    app_secret: F.app_secret || "",
                    chat_ids: Array.isArray(F.chat_ids) ? F.chat_ids.join(",") : ""
                }, W.value = {
                    enabled: !!D.enabled,
                    app_id: D.app_id || "",
                    app_secret: D.app_secret || "",
                    group_ids: D.group_ids || "",
                    direct_ids: D.direct_ids || ""
                }, w.value = {
                    enabled: !!L.enabled,
                    urls: Array.isArray(L.urls) ? L.urls : [],
                    secret: L.secret || "",
                    timeout_ms: L.timeout_ms ?? 5e3,
                    retry_max: L.retry_max ?? 3,
                    text_template: L.text_template ?? "{{device_label}} {{text}}",
                    headers: L.headers && typeof L.headers == "object" ? {
                        ...L.headers
                    } : {}
                };
                const M = u.data.bark || {};
                h.value = {
                    enabled: !!M.enabled,
                    urls: Array.isArray(M.urls) ? M.urls : [],
                    group: M.group || "vohive",
                    icon: M.icon || "",
                    level: M.level || "active"
                };
                const P = u.data.email || {};
                v.value = {
                    enabled: !!P.enabled,
                    use_ssl: !!P.use_ssl,
                    smtp_host: P.smtp_host || "",
                    smtp_port: P.smtp_port ?? null,
                    username: P.username || "",
                    password: P.password || "",
                    from_address: P.from_address || "",
                    to_addresses: Array.isArray(P.to_addresses) ? P.to_addresses.join(",") : ""
                };
                const X = u.data.pushplus || {};
                S.value = {
                    enabled: !!X.enabled,
                    token: X.token || "",
                    topic: X.topic || "",
                    channel: X.channel || "wechat"
                }, C.value = null
            } else C.value = u.error;
            return E.value = !1, u
        }
        async function G(u) {
            V.value = !0;
            const m = await H.saveNotifications(u);
            return m.ok || (C.value = m.error), V.value = !1, m
        }

        function ae() {
            return {
                telegram: {
                    enabled: !!k.value.enabled,
                    bot_token: k.value.bot_token || "",
                    chat_id: k.value.chat_id ? Number(k.value.chat_id) : 0,
                    admin_id: k.value.admin_id ? Number(k.value.admin_id) : 0,
                    base_url: k.value.base_url || "",
                    proxy: k.value.proxy || ""
                },
                feishu: {
                    enabled: !!R.value.enabled,
                    app_id: R.value.app_id || "",
                    app_secret: R.value.app_secret || "",
                    chat_ids: R.value.chat_ids ? R.value.chat_ids.split(",").map(u => u.trim()).filter(Boolean) : []
                },
                qq: {
                    enabled: !!W.value.enabled,
                    app_id: W.value.app_id || "",
                    app_secret: W.value.app_secret || "",
                    group_ids: W.value.group_ids || "",
                    direct_ids: W.value.direct_ids || ""
                },
                email: {
                    enabled: !!v.value.enabled,
                    use_ssl: !!v.value.use_ssl,
                    smtp_host: v.value.smtp_host || "",
                    smtp_port: Number(v.value.smtp_port) || 0,
                    username: v.value.username || "",
                    password: v.value.password || "",
                    from_address: v.value.from_address || "",
                    to_addresses: v.value.to_addresses ? v.value.to_addresses.split(",").map(u => u.trim()).filter(Boolean) : []
                },
                pushplus: {
                    enabled: !!S.value.enabled,
                    token: S.value.token || "",
                    topic: S.value.topic || "",
                    channel: S.value.channel || ""
                },
                webhook: {
                    enabled: !!w.value.enabled,
                    urls: Array.isArray(w.value.urls) ? w.value.urls : [],
                    secret: w.value.secret || "",
                    timeout_ms: Number(w.value.timeout_ms) || 5e3,
                    retry_max: Number(w.value.retry_max) || 3,
                    text_template: String(w.value.text_template || ""),
                    headers: Ee(w.value.headers)
                },
                bark: {
                    enabled: !!h.value.enabled,
                    urls: Array.isArray(h.value.urls) ? h.value.urls : [],
                    group: String(h.value.group || "").trim(),
                    icon: String(h.value.icon || "").trim(),
                    level: String(h.value.level || "").trim()
                }
            }
        }
        async function oe() {
            return G(ae())
        }
        async function re() {
            d.value = !0;
            const u = {
                    enabled: !!w.value.enabled,
                    urls: (Array.isArray(w.value.urls) ? w.value.urls : []).map(F => String(F || "").trim()).filter(Boolean),
                    secret: w.value.secret || "",
                    timeout_ms: Number(w.value.timeout_ms) || 5e3,
                    retry_max: Number(w.value.retry_max) || 3,
                    text_template: String(w.value.text_template || ""),
                    headers: Ee(w.value.headers)
                },
                m = await H.testWebhook(u);
            return m.ok || (C.value = m.error), d.value = !1, m
        }
        async function de() {
            p.value = !0;
            const u = {
                    enabled: !!h.value.enabled,
                    urls: (Array.isArray(h.value.urls) ? h.value.urls : []).map(F => String(F || "").trim()).filter(Boolean),
                    group: String(h.value.group || "").trim(),
                    icon: String(h.value.icon || "").trim(),
                    level: String(h.value.level || "").trim()
                },
                m = await H.testBark(u);
            return m.ok || (C.value = m.error), p.value = !1, m
        }
        async function ne() {
            n.value = !0;
            const u = {
                    enabled: !!v.value.enabled,
                    use_ssl: !!v.value.use_ssl,
                    smtp_host: String(v.value.smtp_host || "").trim(),
                    smtp_port: Number(v.value.smtp_port) || 0,
                    username: String(v.value.username || "").trim(),
                    password: String(v.value.password || "").trim(),
                    from_address: String(v.value.from_address || "").trim(),
                    to_addresses: v.value.to_addresses ? v.value.to_addresses.split(",").map(F => F.trim()).filter(Boolean) : []
                },
                m = await H.testEmail(u);
            return m.ok || (C.value = m.error), n.value = !1, m
        }
        async function Y(u) {
            A.value = !0;
            const m = await H.changePassword(u);
            return m.ok || (C.value = m.error), A.value = !1, m
        }
        async function ie() {
            return Y(U.value)
        }

        function ue() {
            U.value = {
                ...Se
            }
        }
        return {
            systemInfo: _,
            notifications: g,
            passwordForm: U,
            telegramForm: k,
            feishuForm: R,
            qqForm: W,
            webhookSettings: w,
            barkSettings: h,
            emailForm: v,
            pushplusForm: S,
            loadingSystemInfo: y,
            loadingNotifications: E,
            savingNotifications: V,
            testingWebhook: d,
            testingBark: p,
            testingEmail: n,
            changingPassword: A,
            error: C,
            
            fetchNotifications: te,
            saveNotifications: G,
            saveNotificationsFromForms: oe,
            testWebhookFromForm: re,
            testBarkFromForm: de,
            testEmailFromForm: ne,
            changePassword: Y,
            changePasswordFromForm: ie,
            resetPasswordForm: ue
        }
    }),
    is = {
        class: "max-w-5xl mx-auto"
    },
    us = {
        class: "grid grid-cols-1 lg:grid-cols-2 gap-8"
    },
    ps = {
        class: "ui-card p-8 relative overflow-hidden group"
    },
    ms = {
        class: "flex items-center gap-3 mb-6 relative z-10"
    },
    cs = {
        class: "w-12 h-12 rounded-xl bg-indigo-50 dark:bg-indigo-500/10 flex items-center justify-center text-indigo-600 dark:text-indigo-400"
    },
    bs = {
        class: "space-y-4 relative z-10"
    },
    gs = {
        class: "space-y-1"
    },
    vs = {
        class: "space-y-1"
    },
    fs = {
        class: "space-y-1"
    },
    xs = {
        class: "pt-4"
    },
    ys = {
        class: "ui-card p-8 relative overflow-hidden group"
    },
    _s = {
        class: "flex items-center gap-3 mb-6 relative z-10"
    },
    ws = {
        class: "w-12 h-12 rounded-xl bg-indigo-50 dark:bg-indigo-500/10 flex items-center justify-center text-indigo-600 dark:text-indigo-400"
    },
    ks = {
        class: "space-y-4 text-sm relative z-10"
    },
    Vs = {
        class: "p-3 bg-gray-50 dark:bg-white/5 rounded-lg"
    },
    hs = {
        class: "flex items-center justify-end gap-3"
    },
    Us = {
        key: 0,
        class: "p-4 bg-amber-50 dark:bg-amber-500/10 rounded-lg border border-amber-200 dark:border-amber-500/20"
    },
    Ss = {
        class: "flex items-center gap-2 text-amber-800 dark:text-amber-200 mb-2 font-bold text-[13px]"
    },
    Es = {
        class: "text-xs text-amber-700 dark:text-amber-300/80 mb-4 whitespace-pre-wrap max-h-32 overflow-y-auto pr-2 custom-scrollbar"
    },
    As = {
        class: "p-3 bg-gray-50 dark:bg-white/5 rounded-lg"
    },
    Fs = {
        class: "p-3 bg-gray-50 dark:bg-white/5 rounded-lg"
    },
    Ts = {
        class: "p-3 bg-gray-50 dark:bg-white/5 rounded-lg"
    },
    Bs = {
        class: "ui-panel-muted px-4 py-4"
    },
    Is = {
        class: "flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4"
    },
    Ns = {
        class: "min-w-0"
    },
    Rs = {
        class: "flex items-center gap-3"
    },
    Cs = {
        class: "w-9 h-9 rounded-xl bg-indigo-50 dark:bg-indigo-500/10 flex items-center justify-center text-indigo-600 dark:text-indigo-400"
    },
    Ds = {
        class: "notify-card ui-card p-8 relative overflow-hidden group lg:col-span-2"
    },
    Ls = {
        class: "flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6 relative z-10"
    },
    Ps = {
        class: "flex items-center gap-3"
    },
    Os = {
        class: "w-12 h-12 rounded-xl bg-indigo-50 dark:bg-indigo-500/10 flex items-center justify-center text-indigo-600 dark:text-indigo-400"
    },
    Ws = {
        key: 0,
        class: "p-6 text-sm text-gray-500 dark:text-gray-400"
    },
    js = {
        key: 1,
        class: "relative z-10 w-full overflow-hidden"
    },
    zs = {
        class: "flex items-center justify-between mb-4"
    },
    Hs = {
        class: "space-y-4"
    },
    Ms = {
        class: "space-y-1"
    },
    qs = {
        class: "grid grid-cols-1 sm:grid-cols-2 gap-4"
    },
    Qs = {
        class: "space-y-1"
    },
    Ks = {
        class: "space-y-1"
    },
    $s = {
        class: "space-y-1"
    },
    Xs = {
        class: "space-y-1"
    },
    Gs = {
        class: "flex items-center justify-between mb-4"
    },
    Ys = {
        class: "space-y-4"
    },
    Js = {
        class: "grid grid-cols-1 sm:grid-cols-2 gap-4"
    },
    Zs = {
        class: "space-y-1"
    },
    el = {
        class: "space-y-1"
    },
    sl = {
        class: "space-y-1"
    },
    ll = {
        class: "flex items-center justify-between mb-4"
    },
    tl = {
        class: "space-y-4"
    },
    al = {
        class: "grid grid-cols-1 sm:grid-cols-2 gap-4"
    },
    ol = {
        class: "space-y-1"
    },
    rl = {
        class: "space-y-1"
    },
    dl = {
        class: "grid grid-cols-1 sm:grid-cols-2 gap-4"
    },
    nl = {
        class: "space-y-1"
    },
    il = {
        class: "space-y-1"
    },
    ul = {
        class: "flex items-center justify-between mb-4"
    },
    pl = {
        class: "flex items-center gap-2"
    },
    ml = {
        class: "space-y-4"
    },
    cl = {
        class: "space-y-2"
    },
    bl = {
        class: "flex items-center justify-between"
    },
    gl = {
        key: 0,
        class: "text-xs text-gray-400 py-2 border border-dashed border-gray-200 dark:border-white/10 rounded-lg text-center bg-gray-50/30 dark:bg-white/5"
    },
    vl = {
        class: "space-y-1"
    },
    fl = {
        class: "space-y-1"
    },
    xl = {
        class: "space-y-1"
    },
    yl = {
        class: "flex items-center justify-between mb-4"
    },
    _l = {
        class: "flex items-center gap-2"
    },
    wl = {
        class: "space-y-4"
    },
    kl = {
        class: "grid grid-cols-1 sm:grid-cols-10 gap-4"
    },
    Vl = {
        class: "space-y-1 sm:col-span-5"
    },
    hl = {
        class: "space-y-1 sm:col-span-3"
    },
    Ul = {
        class: "space-y-1 sm:col-span-2"
    },
    Sl = {
        class: "h-10 flex items-center"
    },
    El = {
        class: "grid grid-cols-1 sm:grid-cols-2 gap-4"
    },
    Al = {
        class: "space-y-1"
    },
    Fl = {
        class: "space-y-1"
    },
    Tl = {
        class: "space-y-1"
    },
    Bl = {
        class: "space-y-1"
    },
    Il = {
        class: "flex items-center justify-between mb-4"
    },
    Nl = {
        class: "space-y-4"
    },
    Rl = {
        class: "space-y-1"
    },
    Cl = {
        class: "space-y-1"
    },
    Dl = {
        class: "space-y-1"
    },
    Ll = {
        class: "flex items-center justify-between mb-4"
    },
    Pl = {
        class: "flex items-center gap-2"
    },
    Ol = {
        class: "space-y-4"
    },
    Wl = {
        class: "space-y-2"
    },
    jl = {
        class: "flex items-center justify-between"
    },
    zl = {
        key: 0,
        class: "text-xs text-gray-400 py-2 border border-dashed border-gray-200 dark:border-white/10 rounded-lg text-center bg-gray-50/30 dark:bg-white/5"
    },
    Hl = {
        class: "space-y-1"
    },
    Ml = {
        class: "space-y-2"
    },
    ql = {
        class: "flex items-center justify-between"
    },
    Ql = {
        key: 0,
        class: "text-xs text-gray-400 py-2 border border-dashed border-gray-200 dark:border-white/10 rounded-lg text-center bg-gray-50/30 dark:bg-white/5"
    },
    Kl = {
        class: "space-y-1"
    },
    $l = {
        class: "grid grid-cols-1 sm:grid-cols-2 gap-4"
    },
    Xl = {
        class: "space-y-1"
    },
    Gl = {
        class: "space-y-1"
    },
    Yl = Re({
        __name: "Settings",
        setup(_) {
            const g = ns(),
                {
                    systemInfo: U,
                    loadingNotifications: k,
                    savingNotifications: R,
                    testingWebhook: W,
                    testingBark: w,
                    testingEmail: h,
                    changingPassword: v,
                    passwordForm: S,
                    telegramForm: y,
                    feishuForm: E,
                    qqForm: V,
                    webhookSettings: d,
                    barkSettings: p,
                    emailForm: n,
                    pushplusForm: A
                } = Oe(g),
                C = b("telegram"),
                le = ge(() => Array.isArray(d.value.urls) ? d.value.urls.some(o => String(o || "").trim().length > 0) : !1),
                te = ge(() => Array.isArray(p.value.urls) ? p.value.urls.some(o => String(o || "").trim().length > 0) : !1),
                G = ge(() => !!(n.value.smtp_host && n.value.smtp_port && n.value.username && n.value.password && n.value.from_address && n.value.to_addresses));
            async function ae() {
                if (S.value.new_password !== S.value.confirm_password) {
                    c.error("两次输入的新密码不一致");
                    return
                }
                try {
                    const o = await g.changePasswordFromForm();
                    if (!o.ok) throw new Error(o.error.message || "更新失败");
                    c.success("密码已更新"), g.resetPasswordForm()
                } catch {
                    c.error("失败：后端尚未实现该功能或请求失败")
                }
            }
            async function re() {
                try {
                    const o = await g.fetchNotifications();
                    if (!o.ok) throw new Error(o.error.message || "通知配置加载失败");
                    L()
                } catch {
                    c.error("通知配置加载失败")
                }
            }

            async function ne() {
                try {
                    const o = await g.saveNotificationsFromForms();
                    if (!o.ok) throw new Error(o.error.message || "通知配置保存失败");
                    const e = o.data.applied,
                        f = o.data.warning;
                    e === !1 && f ? c.warning(f) : c.success("通知配置已保存（已写入 config.yaml）")
                } catch (o) {
                    c.error(o instanceof Error ? o.message : "通知配置保存失败")
                }
            }
            async function Y() {
                try {
                    const o = await g.testWebhookFromForm();
                    if (!o.ok) throw new Error(o.error.message || "Webhook 测试失败");
                    const e = o.data;
                    if (e.ok) {
                        c.success(e.message || "测试通知已发送");
                        return
                    }
                    if (Array.isArray(e.failed_urls) && e.failed_urls.length > 0) {
                        c.error(`${e.message}
失败 URL: ${e.failed_urls.join(", ")}`);
                        return
                    }
                    c.error(e.message || "Webhook 测试失败")
                } catch (o) {
                    c.error(o instanceof Error ? o.message : "Webhook 测试失败")
                }
            }

            function ie() {
                d.value.urls || (d.value.urls = []), d.value.urls.push("")
            }

            function ue(o) {
                d.value.urls.splice(o, 1)
            }
            const u = new Set(["content-type", "x-vohive-signature"]),
                m = ["Authorization", "X-Api-Key", "X-Auth-Token", "X-Webhook-Token", "X-Signature", "X-Request-Id", "Accept", "User-Agent"];
            let F = 0;
            const D = b([]);

            function L() {
                const o = d.value.headers || {};
                D.value = Object.entries(o).map(([e, f]) => ({
                    id: F++,
                    key: e,
                    value: String(f ?? "")
                }))
            }
            we(D, o => {
                const e = {};
                for (const f of o) {
                    const i = String(f.key || "").trim();
                    !i || u.has(i.toLowerCase()) || (e[i] = String(f.value ?? ""))
                }
                d.value.headers = e
            }, {
                deep: !0
            });

            function M() {
                D.value.push({
                    id: F++,
                    key: "",
                    value: ""
                })
            }

            function P(o) {
                D.value.splice(o, 1)
            }
            async function X() {
                try {
                    const o = await g.testBarkFromForm();
                    if (!o.ok) throw new Error(o.error.message || "Bark 测试失败");
                    const e = o.data;
                    if (e.ok) {
                        c.success(e.message || "测试通知已发送");
                        return
                    }
                    if (Array.isArray(e.failed_urls) && e.failed_urls.length > 0) {
                        c.error(`${e.message}
失败 URL: ${e.failed_urls.join(", ")}`);
                        return
                    }
                    c.error(e.message || "Bark 测试失败")
                } catch (o) {
                    c.error(o instanceof Error ? o.message : "Bark 测试失败")
                }
            }
            async function Ae() {
                try {
                    const o = await g.testEmailFromForm();
                    if (!o.ok) throw new Error(o.error.message || "Email 测试失败");
                    const e = o.data;
                    if (e.ok) {
                        c.success(e.message || "测试邮件已发送");
                        return
                    }
                    c.error(e.message || "Email 测试失败")
                } catch (o) {
                    c.error(o instanceof Error ? o.message : "Email 测试失败")
                }
            }

            function Fe() {
                p.value.urls || (p.value.urls = []), p.value.urls.push("")
            }

            function Te(o) {
                p.value.urls.splice(o, 1)
            }
            we(() => n.value.smtp_port, o => {
                Number(o) === 465 && (n.value.use_ssl = !0)
            });
            return Ce(() => {
                re()
            }), De(() => {}), (o, e) => {
                var ye, _e;
                const f = ze,
                    i = He,
                    T = Me,
                    Q = Ke,
                    $ = Qe,
                    K = Xe,
                    ce = $e,
                    xe = Ge,
                    Ne = qe;
                return N(), I("div", is, [t(We, {
                    title: "系统设置",
                    subtitle: "管理网关参数与运行信息"
                }), s("div", us, [s("div", ps, [e[45] || (e[45] = s("div", {
                    class: "absolute top-0 right-0 w-40 h-40 bg-indigo-500/5 rounded-bl-full -mr-10 -mt-10 transition-transform group-hover:scale-110"
                }, null, -1)), s("div", ms, [s("div", cs, [t(f, {
                    size: "24"
                }, {
                    default: r(() => [t(l(Ye))]),
                    _: 1
                })]), e[40] || (e[40] = s("div", null, [s("h3", {
                    class: "text-lg font-bold text-gray-800 dark:text-gray-100"
                }, "安全"), s("p", {
                    class: "text-xs text-gray-500"
                }, "更新访问凭证")], -1))]), s("div", bs, [s("div", gs, [e[41] || (e[41] = s("label", {
                    class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                }, "当前密码", -1)), t(i, {
                    modelValue: l(S).old_password,
                    "onUpdate:modelValue": e[0] || (e[0] = a => l(S).old_password = a),
                    type: "password",
                    "show-password": "",
                    placeholder: "••••••••",
                    size: "large"
                }, null, 8, ["modelValue"])]), s("div", vs, [e[42] || (e[42] = s("label", {
                    class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                }, "新密码", -1)), t(i, {
                    modelValue: l(S).new_password,
                    "onUpdate:modelValue": e[1] || (e[1] = a => l(S).new_password = a),
                    type: "password",
                    "show-password": "",
                    placeholder: "••••••••",
                    size: "large"
                }, null, 8, ["modelValue"])]), s("div", fs, [e[43] || (e[43] = s("label", {
                    class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                }, "确认新密码", -1)), t(i, {
                    modelValue: l(S).confirm_password,
                    "onUpdate:modelValue": e[2] || (e[2] = a => l(S).confirm_password = a),
                    type: "password",
                    "show-password": "",
                    placeholder: "••••••••",
                    size: "large"
                }, null, 8, ["modelValue"])]), s("div", xs, [t(T, {
                    type: "primary",
                    loading: l(v),
                    onClick: ae,
                    size: "large",
                    class: "w-full !border-0"
                }, {
                    default: r(() => [t(f, null, {
                        default: r(() => [t(l(Ve))]),
                        _: 1
                    }), e[44] || (e[44] = x(" 更新凭证 ", -1))]),
                    _: 1
                }, 8, ["loading"])])])]), s("section", ys, [s("h3", {class: "text-lg font-bold text-gray-800 dark:text-gray-100"}, "API 文档"), s("a", {href: "/api/docs", target: "_blank", rel: "noopener noreferrer", class: "inline-block text-sm text-indigo-600 hover:underline mt-2"}, "查看本机接口说明")]), s("div", Ds, [e[109] || (e[109] = s("div", {
                    class: "absolute top-0 right-0 w-40 h-40 bg-indigo-500/5 rounded-bl-full -mr-10 -mt-10 transition-transform group-hover:scale-110"
                }, null, -1)), s("div", Ls, [s("div", Ps, [s("div", Os, [t(f, {
                    size: "24"
                }, {
                    default: r(() => [t(l(he))]),
                    _: 1
                })]), e[52] || (e[52] = s("div", null, [s("h3", {
                    class: "text-lg font-bold text-gray-800 dark:text-gray-100"
                }, "通知"), s("p", {
                    class: "text-xs text-gray-500"
                }, "Telegram / 飞书 / QQ / Webhook")], -1))]), t(T, {
                    type: "primary",
                    loading: l(R),
                    disabled: l(k),
                    onClick: ne,
                    class: "!border-0"
                }, {
                    default: r(() => [t(f, null, {
                        default: r(() => [t(l(Ve))]),
                        _: 1
                    }), e[53] || (e[53] = x(" 保存通知配置 ", -1))]),
                    _: 1
                }, 8, ["loading", "disabled"])]), l(k) ? (N(), I("div", Ws, "正在加载通知配置…")) : (N(), I("div", js, [t(Ne, {
                    modelValue: C.value,
                    "onUpdate:modelValue": e[39] || (e[39] = a => C.value = a),
                    class: "settings-notify-tabs"
                }, {
                    default: r(() => [t($, {
                        label: "Telegram Bot",
                        name: "telegram",
                        class: "pt-2"
                    }, {
                        default: r(() => [s("div", zs, [e[54] || (e[54] = s("div", {
                            class: "flex items-center gap-2"
                        }, [s("div", {
                            class: "font-bold text-gray-800 dark:text-gray-100"
                        }, "启用 Telegram 机器人")], -1)), t(Q, {
                            modelValue: l(y).enabled,
                            "onUpdate:modelValue": e[3] || (e[3] = a => l(y).enabled = a)
                        }, null, 8, ["modelValue"])]), s("div", Hs, [s("div", Ms, [e[55] || (e[55] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "Bot Token", -1)), t(i, {
                            modelValue: l(y).bot_token,
                            "onUpdate:modelValue": e[4] || (e[4] = a => l(y).bot_token = a),
                            disabled: !l(y).enabled,
                            placeholder: "xxxx:yyyy"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", qs, [s("div", Qs, [e[56] || (e[56] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "Chat ID", -1)), t(i, {
                            modelValue: l(y).chat_id,
                            "onUpdate:modelValue": e[5] || (e[5] = a => l(y).chat_id = a),
                            disabled: !l(y).enabled,
                            type: "number",
                            inputmode: "numeric",
                            placeholder: "例如 123456"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", Ks, [e[57] || (e[57] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "Admin ID", -1)), t(i, {
                            modelValue: l(y).admin_id,
                            "onUpdate:modelValue": e[6] || (e[6] = a => l(y).admin_id = a),
                            disabled: !l(y).enabled,
                            type: "number",
                            inputmode: "numeric",
                            placeholder: "例如 123456"
                        }, null, 8, ["modelValue", "disabled"])])]), s("div", $s, [e[58] || (e[58] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "TG API 反代（可选）", -1)), t(i, {
                            modelValue: l(y).base_url,
                            "onUpdate:modelValue": e[7] || (e[7] = a => l(y).base_url = a),
                            disabled: !l(y).enabled,
                            placeholder: "留空直连 api.telegram.org；需要反代时填写"
                        }, null, 8, ["modelValue", "disabled"]), e[59] || (e[59] = s("div", {
                            class: "text-[10px] text-gray-400 mt-1"
                        }, "反向代理地址 (例如 https://api.telegram.org/bot%s/%s)", -1))]), s("div", Xs, [e[60] || (e[60] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "HTTP 代理（可选）", -1)), t(i, {
                            modelValue: l(y).proxy,
                            "onUpdate:modelValue": e[8] || (e[8] = a => l(y).proxy = a),
                            disabled: !l(y).enabled,
                            placeholder: "例如 http://127.0.0.1:7890"
                        }, null, 8, ["modelValue", "disabled"]), e[61] || (e[61] = s("div", {
                            class: "text-[10px] text-gray-400 mt-1"
                        }, "用于连接 API 服务器的 HTTP 代理", -1))])])]),
                        _: 1
                    }), t($, {
                        label: "飞书 Bot",
                        name: "feishu",
                        class: "pt-2"
                    }, {
                        default: r(() => [s("div", Gs, [e[62] || (e[62] = s("div", {
                            class: "flex items-center gap-2"
                        }, [s("div", {
                            class: "font-bold text-gray-800 dark:text-gray-100"
                        }, "启用飞书机器人")], -1)), t(Q, {
                            modelValue: l(E).enabled,
                            "onUpdate:modelValue": e[9] || (e[9] = a => l(E).enabled = a)
                        }, null, 8, ["modelValue"])]), s("div", Ys, [s("div", Js, [s("div", Zs, [e[63] || (e[63] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "App ID", -1)), t(i, {
                            modelValue: l(E).app_id,
                            "onUpdate:modelValue": e[10] || (e[10] = a => l(E).app_id = a),
                            disabled: !l(E).enabled,
                            placeholder: "cli_xxxx"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", el, [e[64] || (e[64] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "App Secret", -1)), t(i, {
                            modelValue: l(E).app_secret,
                            "onUpdate:modelValue": e[11] || (e[11] = a => l(E).app_secret = a),
                            disabled: !l(E).enabled,
                            type: "password",
                            "show-password": "",
                            placeholder: "••••••••"
                        }, null, 8, ["modelValue", "disabled"])])]), s("div", sl, [e[65] || (e[65] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "Chat IDs", -1)), t(i, {
                            modelValue: l(E).chat_ids,
                            "onUpdate:modelValue": e[12] || (e[12] = a => l(E).chat_ids = a),
                            disabled: !l(E).enabled,
                            placeholder: "多个群组用英文逗号分隔"
                        }, null, 8, ["modelValue", "disabled"]), e[66] || (e[66] = s("div", {
                            class: "text-[10px] text-gray-400 mt-1"
                        }, "飞书群聊的 Chat ID (oc_xxxx)，可通过飞书开放平台 API 获取，支持逗号分隔多个群组。", -1))]), e[67] || (e[67] = s("div", {
                            class: "p-3 rounded-xl bg-indigo-50/50 dark:bg-indigo-500/5 text-xs text-indigo-600 dark:text-indigo-400/80 leading-relaxed border border-indigo-100/50 dark:border-indigo-500/10"
                        }, [s("strong", null, "配置说明："), s("ol", {
                            class: "list-decimal ml-4 mt-1 space-y-1"
                        }, [s("li", null, [x("在"), s("a", {
                            href: "https://open.feishu.cn",
                            target: "_blank",
                            class: "underline hover:text-indigo-700"
                        }, "飞书开放平台"), x("创建自建应用，启用「机器人」能力")]), s("li", null, "在「事件与回调 → 事件配置」中选择「使用长连接接收事件」"), s("li", null, [x("添加 "), s("code", null, "im:message"), x(" 和 "), s("code", null, "im:message:send_as_bot"), x(" 权限")])])], -1))])]),
                        _: 1
                    }), t($, {
                        label: "QQ Bot",
                        name: "qq",
                        class: "pt-2"
                    }, {
                        default: r(() => [s("div", ll, [e[68] || (e[68] = s("div", {
                            class: "flex items-center gap-2"
                        }, [s("div", {
                            class: "font-bold text-gray-800 dark:text-gray-100"
                        }, "启用 QQ 机器人")], -1)), t(Q, {
                            modelValue: l(V).enabled,
                            "onUpdate:modelValue": e[13] || (e[13] = a => l(V).enabled = a)
                        }, null, 8, ["modelValue"])]), s("div", tl, [s("div", al, [s("div", ol, [e[69] || (e[69] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "App ID", -1)), t(i, {
                            modelValue: l(V).app_id,
                            "onUpdate:modelValue": e[14] || (e[14] = a => l(V).app_id = a),
                            disabled: !l(V).enabled,
                            placeholder: "QQ Bot App ID"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", rl, [e[70] || (e[70] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "App Secret", -1)), t(i, {
                            modelValue: l(V).app_secret,
                            "onUpdate:modelValue": e[15] || (e[15] = a => l(V).app_secret = a),
                            disabled: !l(V).enabled,
                            type: "password",
                            "show-password": "",
                            placeholder: "••••••••"
                        }, null, 8, ["modelValue", "disabled"])])]), s("div", dl, [s("div", nl, [e[71] || (e[71] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "Group IDs (群聊)", -1)), t(i, {
                            modelValue: l(V).group_ids,
                            "onUpdate:modelValue": e[16] || (e[16] = a => l(V).group_ids = a),
                            disabled: !l(V).enabled,
                            placeholder: "群聊 OpenID，多个使用逗号分隔"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", il, [e[72] || (e[72] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "User IDs (私聊)", -1)), t(i, {
                            modelValue: l(V).direct_ids,
                            "onUpdate:modelValue": e[17] || (e[17] = a => l(V).direct_ids = a),
                            disabled: !l(V).enabled,
                            placeholder: "用户 OpenID，多个使用逗号分隔"
                        }, null, 8, ["modelValue", "disabled"])])]), e[73] || (e[73] = s("div", {
                            class: "p-3 rounded-xl bg-amber-50/50 dark:bg-amber-500/5 text-xs text-amber-700 dark:text-amber-400/80 leading-relaxed border border-amber-100/50 dark:border-amber-500/10"
                        }, [s("ol", {
                            class: "list-decimal ml-4 mt-1 space-y-1"
                        }, [s("li", null, [x("QQbot申请地址："), s("a", {
                            href: "https://q.qq.com/qqbot/openclaw/index.html",
                            target: "_blank",
                            class: "underline hover:text-amber-800"
                        }, "官方控制台")]), s("li", null, "向机器人发送消息后，去系统日志查看 OpenID，填入后 Bot 只对匹配的会话进行回复和推送。")])], -1))])]),
                        _: 1
                    }), t($, {
                        label: "Bark",
                        name: "bark",
                        class: "pt-2"
                    }, {
                        default: r(() => [s("div", ul, [e[75] || (e[75] = s("div", {
                            class: "flex items-center gap-2"
                        }, [s("div", {
                            class: "font-bold text-gray-800 dark:text-gray-100"
                        }, "启用 Bark 推送")], -1)), s("div", pl, [t(T, {
                            size: "small",
                            type: "primary",
                            plain: "",
                            loading: l(w),
                            disabled: !l(p).enabled || !te.value,
                            onClick: X
                        }, {
                            default: r(() => [...e[74] || (e[74] = [x(" 测试通知 ", -1)])]),
                            _: 1
                        }, 8, ["loading", "disabled"]), t(Q, {
                            modelValue: l(p).enabled,
                            "onUpdate:modelValue": e[18] || (e[18] = a => l(p).enabled = a)
                        }, null, 8, ["modelValue"])])]), s("div", ml, [s("div", cl, [s("div", bl, [e[77] || (e[77] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "目标 URLs", -1)), t(T, {
                            size: "small",
                            type: "primary",
                            plain: "",
                            onClick: Fe,
                            disabled: !l(p).enabled
                        }, {
                            default: r(() => [t(f, null, {
                                default: r(() => [t(l(ve))]),
                                _: 1
                            }), e[76] || (e[76] = s("span", {
                                class: "ml-1"
                            }, "添加 URL", -1))]),
                            _: 1
                        }, 8, ["disabled"])]), l(p).urls && l(p).urls.length === 0 ? (N(), I("div", gl, " 尚未配置任何 Bark URL，点击右侧添加按钮。 ")) : J("", !0), (N(!0), I(Z, null, ee(l(p).urls, (a, O) => (N(), I("div", {
                            key: O,
                            class: "flex items-center gap-2"
                        }, [t(i, {
                            modelValue: l(p).urls[O],
                            "onUpdate:modelValue": B => l(p).urls[O] = B,
                            disabled: !l(p).enabled,
                            placeholder: "https://api.day.app/YOUR_KEY/",
                            class: "flex-1"
                        }, null, 8, ["modelValue", "onUpdate:modelValue", "disabled"]), t(T, {
                            type: "danger",
                            plain: "",
                            onClick: B => Te(O),
                            disabled: !l(p).enabled
                        }, {
                            default: r(() => [t(f, null, {
                                default: r(() => [t(l(fe))]),
                                _: 1
                            })]),
                            _: 1
                        }, 8, ["onClick", "disabled"])]))), 128))]), s("div", vl, [e[78] || (e[78] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "分组 (Group)", -1)), t(i, {
                            modelValue: l(p).group,
                            "onUpdate:modelValue": e[19] || (e[19] = a => l(p).group = a),
                            disabled: !l(p).enabled,
                            placeholder: "例如 vohive"
                        }, null, 8, ["modelValue", "disabled"]), e[79] || (e[79] = s("div", {
                            class: "text-[10px] text-gray-400 mt-1"
                        }, "iOS 设备上的通知分组。", -1))]), s("div", fl, [e[80] || (e[80] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "通知级别 (Level)", -1)), t(ce, {
                            modelValue: l(p).level,
                            "onUpdate:modelValue": e[20] || (e[20] = a => l(p).level = a),
                            disabled: !l(p).enabled,
                            placeholder: "选择通知级别",
                            class: "w-full"
                        }, {
                            default: r(() => [t(K, {
                                label: "时效性 (timeSensitive)",
                                value: "timeSensitive"
                            }), t(K, {
                                label: "积极 (active)",
                                value: "active"
                            }), t(K, {
                                label: "被动 (passive)",
                                value: "passive"
                            })]),
                            _: 1
                        }, 8, ["modelValue", "disabled"]), e[81] || (e[81] = s("div", {
                            class: "text-[10px] text-gray-400 mt-1"
                        }, "iOS 的专注模式/打扰规则会根据此级别决定是否亮屏。", -1))]), s("div", xl, [e[82] || (e[82] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "图标 (Icon)", -1)), t(i, {
                            modelValue: l(p).icon,
                            "onUpdate:modelValue": e[21] || (e[21] = a => l(p).icon = a),
                            disabled: !l(p).enabled,
                            placeholder: "图标 URL，可选"
                        }, null, 8, ["modelValue", "disabled"])])])]),
                        _: 1
                    }), t($, {
                        label: "Email",
                        name: "email",
                        class: "pt-2"
                    }, {
                        default: r(() => [s("div", yl, [e[84] || (e[84] = s("div", {
                            class: "flex items-center gap-2"
                        }, [s("div", {
                            class: "font-bold text-gray-800 dark:text-gray-100"
                        }, "启用 Email 推送")], -1)), s("div", _l, [t(T, {
                            size: "small",
                            type: "primary",
                            plain: "",
                            loading: l(h),
                            disabled: !l(n).enabled || !G.value,
                            onClick: Ae
                        }, {
                            default: r(() => [...e[83] || (e[83] = [x(" 测试通知 ", -1)])]),
                            _: 1
                        }, 8, ["loading", "disabled"]), t(Q, {
                            modelValue: l(n).enabled,
                            "onUpdate:modelValue": e[22] || (e[22] = a => l(n).enabled = a)
                        }, null, 8, ["modelValue"])])]), s("div", wl, [s("div", kl, [s("div", Vl, [e[85] || (e[85] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "SMTP 主机", -1)), t(i, {
                            modelValue: l(n).smtp_host,
                            "onUpdate:modelValue": e[23] || (e[23] = a => l(n).smtp_host = a),
                            disabled: !l(n).enabled,
                            placeholder: "smtp.example.com"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", hl, [e[86] || (e[86] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "SMTP 端口", -1)), t(i, {
                            modelValue: l(n).smtp_port,
                            "onUpdate:modelValue": e[24] || (e[24] = a => l(n).smtp_port = a),
                            disabled: !l(n).enabled,
                            type: "number",
                            inputmode: "numeric",
                            placeholder: "465 / 587"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", Ul, [e[87] || (e[87] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider block"
                        }, "使用 SSL/TLS ", -1)), s("div", Sl, [t(Q, {
                            modelValue: l(n).use_ssl,
                            "onUpdate:modelValue": e[25] || (e[25] = a => l(n).use_ssl = a),
                            disabled: !l(n).enabled
                        }, null, 8, ["modelValue", "disabled"])])])]), s("div", El, [s("div", Al, [e[88] || (e[88] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "用户名 (Username)", -1)), t(i, {
                            modelValue: l(n).username,
                            "onUpdate:modelValue": e[26] || (e[26] = a => l(n).username = a),
                            disabled: !l(n).enabled,
                            placeholder: "邮箱账号"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", Fl, [e[89] || (e[89] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "密码 (Password)", -1)), t(i, {
                            modelValue: l(n).password,
                            "onUpdate:modelValue": e[27] || (e[27] = a => l(n).password = a),
                            disabled: !l(n).enabled,
                            type: "password",
                            "show-password": "",
                            placeholder: "邮箱密码或授权码"
                        }, null, 8, ["modelValue", "disabled"])])]), s("div", Tl, [e[90] || (e[90] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "发件人地址 (From)", -1)), t(i, {
                            modelValue: l(n).from_address,
                            "onUpdate:modelValue": e[28] || (e[28] = a => l(n).from_address = a),
                            disabled: !l(n).enabled,
                            placeholder: "例如 noreply@example.com"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", Bl, [e[91] || (e[91] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "收件人地址 (To)", -1)), t(i, {
                            modelValue: l(n).to_addresses,
                            "onUpdate:modelValue": e[29] || (e[29] = a => l(n).to_addresses = a),
                            disabled: !l(n).enabled,
                            placeholder: "多个收件人请用英文逗号分隔"
                        }, null, 8, ["modelValue", "disabled"])])])]),
                        _: 1
                    }), t($, {
                        label: "Pushplus",
                        name: "pushplus",
                        class: "pt-2"
                    }, {
                        default: r(() => [s("div", Il, [e[92] || (e[92] = s("div", {
                            class: "flex items-center gap-2"
                        }, [s("div", {
                            class: "font-bold text-gray-800 dark:text-gray-100"
                        }, "启用 Pushplus 推送")], -1)), t(Q, {
                            modelValue: l(A).enabled,
                            "onUpdate:modelValue": e[30] || (e[30] = a => l(A).enabled = a)
                        }, null, 8, ["modelValue"])]), s("div", Nl, [s("div", Rl, [e[93] || (e[93] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "Token", -1)), t(i, {
                            modelValue: l(A).token,
                            "onUpdate:modelValue": e[31] || (e[31] = a => l(A).token = a),
                            disabled: !l(A).enabled,
                            placeholder: "Pushplus 用户的 Token"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", Cl, [e[94] || (e[94] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "群组编码 (Topic)", -1)), t(i, {
                            modelValue: l(A).topic,
                            "onUpdate:modelValue": e[32] || (e[32] = a => l(A).topic = a),
                            disabled: !l(A).enabled,
                            placeholder: "群组编码，不填则发给个人"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", Dl, [e[95] || (e[95] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "渠道 (Channel)", -1)), t(ce, {
                            modelValue: l(A).channel,
                            "onUpdate:modelValue": e[33] || (e[33] = a => l(A).channel = a),
                            disabled: !l(A).enabled,
                            placeholder: "选择渠道",
                            class: "w-full"
                        }, {
                            default: r(() => [t(K, {
                                label: "微信 (wechat)",
                                value: "wechat"
                            }), t(K, {
                                label: "Webhook (webhook)",
                                value: "webhook"
                            }), t(K, {
                                label: "企业微信 (cp)",
                                value: "cp"
                            }), t(K, {
                                label: "邮件 (mail)",
                                value: "mail"
                            })]),
                            _: 1
                        }, 8, ["modelValue", "disabled"])])])]),
                        _: 1
                    }), t($, {
                        label: "Webhook",
                        name: "webhook",
                        class: "pt-2"
                    }, {
                        default: r(() => [s("div", Ll, [e[97] || (e[97] = s("div", {
                            class: "flex items-center gap-2"
                        }, [s("div", {
                            class: "font-bold text-gray-800 dark:text-gray-100"
                        }, "启用 Webhook 推送")], -1)), s("div", Pl, [t(T, {
                            size: "small",
                            type: "primary",
                            plain: "",
                            loading: l(W),
                            disabled: !l(d).enabled || !le.value,
                            onClick: Y
                        }, {
                            default: r(() => [...e[96] || (e[96] = [x(" 测试通知 ", -1)])]),
                            _: 1
                        }, 8, ["loading", "disabled"]), t(Q, {
                            modelValue: l(d).enabled,
                            "onUpdate:modelValue": e[34] || (e[34] = a => l(d).enabled = a)
                        }, null, 8, ["modelValue"])])]), s("div", Ol, [s("div", Wl, [s("div", jl, [e[99] || (e[99] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "目标 URLs", -1)), t(T, {
                            size: "small",
                            type: "primary",
                            plain: "",
                            onClick: ie,
                            disabled: !l(d).enabled
                        }, {
                            default: r(() => [t(f, null, {
                                default: r(() => [t(l(ve))]),
                                _: 1
                            }), e[98] || (e[98] = s("span", {
                                class: "ml-1"
                            }, "添加 URL", -1))]),
                            _: 1
                        }, 8, ["disabled"])]), l(d).urls && l(d).urls.length === 0 ? (N(), I("div", zl, " 尚未配置任何 Webhook URL，点击右侧添加按钮。 ")) : J("", !0), (N(!0), I(Z, null, ee(l(d).urls, (a, O) => (N(), I("div", {
                            key: O,
                            class: "flex items-center gap-2"
                        }, [t(i, {
                            modelValue: l(d).urls[O],
                            "onUpdate:modelValue": B => l(d).urls[O] = B,
                            disabled: !l(d).enabled,
                            placeholder: "https://...",
                            class: "flex-1"
                        }, null, 8, ["modelValue", "onUpdate:modelValue", "disabled"]), t(T, {
                            type: "danger",
                            plain: "",
                            onClick: B => ue(O),
                            disabled: !l(d).enabled
                        }, {
                            default: r(() => [t(f, null, {
                                default: r(() => [t(l(fe))]),
                                _: 1
                            })]),
                            _: 1
                        }, 8, ["onClick", "disabled"])]))), 128))]), s("div", Hl, [e[100] || (e[100] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "数字签名密钥 (Secret)", -1)), t(i, {
                            modelValue: l(d).secret,
                            "onUpdate:modelValue": e[35] || (e[35] = a => l(d).secret = a),
                            disabled: !l(d).enabled,
                            placeholder: "用于 HMAC-SHA256 签名，选填"
                        }, null, 8, ["modelValue", "disabled"]), e[101] || (e[101] = s("div", {
                            class: "text-[10px] text-gray-400 mt-1"
                        }, "若配置，将通过请求头 X-Vohive-Signature 提供 payload 验证。", -1))]), s("div", Ml, [s("div", ql, [e[103] || (e[103] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "自定义请求头 (Headers)", -1)), t(T, {
                            size: "small",
                            type: "primary",
                            plain: "",
                            onClick: M,
                            disabled: !l(d).enabled
                        }, {
                            default: r(() => [t(f, null, {
                                default: r(() => [t(l(ve))]),
                                _: 1
                            }), e[102] || (e[102] = s("span", {
                                class: "ml-1"
                            }, "添加 Header", -1))]),
                            _: 1
                        }, 8, ["disabled"])]), D.value.length === 0 ? (N(), I("div", Ql, " 尚未配置自定义请求头，例如 Authorization、X-Api-Key 等。 ")) : J("", !0), (N(!0), I(Z, null, ee(D.value, (a, O) => (N(), I("div", {
                            key: a.id,
                            class: "flex items-center gap-2"
                        }, [t(ce, {
                            modelValue: a.key,
                            "onUpdate:modelValue": B => a.key = B,
                            disabled: !l(d).enabled,
                            filterable: "",
                            "allow-create": "",
                            "default-first-option": "",
                            placeholder: "选择或输入 Header 名",
                            class: "flex-1"
                        }, {
                            default: r(() => [(N(), I(Z, null, ee(m, B => t(K, {
                                key: B,
                                label: B,
                                value: B
                            }, null, 8, ["label", "value"])), 64))]),
                            _: 1
                        }, 8, ["modelValue", "onUpdate:modelValue", "disabled"]), t(i, {
                            modelValue: a.value,
                            "onUpdate:modelValue": B => a.value = B,
                            disabled: !l(d).enabled,
                            placeholder: "值，如 Bearer xxx",
                            class: "flex-1"
                        }, null, 8, ["modelValue", "onUpdate:modelValue", "disabled"]), t(T, {
                            type: "danger",
                            plain: "",
                            onClick: B => P(O),
                            disabled: !l(d).enabled
                        }, {
                            default: r(() => [t(f, null, {
                                default: r(() => [t(l(fe))]),
                                _: 1
                            })]),
                            _: 1
                        }, 8, ["onClick", "disabled"])]))), 128)), e[104] || (e[104] = s("div", {
                            class: "text-[10px] text-gray-400 mt-1"
                        }, " Content-Type 与 X-Vohive-Signature 为系统保留头，自定义同名头会被忽略。 ", -1))]), s("div", Kl, [e[105] || (e[105] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "文本模板 (Text Template)", -1)), t(i, {
                            modelValue: l(d).text_template,
                            "onUpdate:modelValue": e[36] || (e[36] = a => l(d).text_template = a),
                            disabled: !l(d).enabled,
                            type: "textarea",
                            rows: 2,
                            placeholder: "{{device_label}} {{text}}"
                        }, null, 8, ["modelValue", "disabled"]), e[106] || (e[106] = s("div", {
                            class: "text-[10px] text-gray-400 mt-1"
                        }, [x(" 支持占位符："), s("code", null, "{{text}}"), x("、"), s("code", null, "{{event}}"), x("、"), s("code", null, "{{timestamp}}"), x("、"), s("code", null, "{{device_id}}"), x("、"), s("code", null, "{{device_name}}"), x("、"), s("code", null, "{{device_label}}"), x("。留空则直接发送原始 text。 ")], -1))]), s("div", $l, [s("div", Xl, [e[107] || (e[107] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "请求超时 (ms)", -1)), t(xe, {
                            modelValue: l(d).timeout_ms,
                            "onUpdate:modelValue": e[37] || (e[37] = a => l(d).timeout_ms = a),
                            min: 1e3,
                            max: 6e4,
                            disabled: !l(d).enabled,
                            class: "w-full !w-full",
                            "controls-position": "right"
                        }, null, 8, ["modelValue", "disabled"])]), s("div", Gl, [e[108] || (e[108] = s("label", {
                            class: "text-xs font-bold text-gray-500 uppercase tracking-wider"
                        }, "最大重试次数", -1)), t(xe, {
                            modelValue: l(d).retry_max,
                            "onUpdate:modelValue": e[38] || (e[38] = a => l(d).retry_max = a),
                            min: 0,
                            max: 10,
                            disabled: !l(d).enabled,
                            class: "w-full !w-full",
                            "controls-position": "right"
                        }, null, 8, ["modelValue", "disabled"])])])])]),
                        _: 1
                    })]),
                    _: 1
                }, 8, ["modelValue"])]))])])])
            }
        }
    }),
    ot = je(Yl, [
        ["__scopeId", "data-v-e521dcd2"]
    ]);
export {
    ot as
    default
};