// Fetch full, current configurations so changing one switch preserves credentials
// and configuration fields on every other instance.
export async function setOutboundEnabled(api,id,enabled){
 const requireOk=result=>{if(!result.ok)throw Error(result.error?.message||'更新代理失败');return result.data;};
 const overview=requireOk(await api.fetchOverview());
 const rows=await Promise.all((overview.instances||[]).map(async row=>requireOk(await api.fetchInstance(row.id))));
 const target=rows.find(row=>row.id===id);if(!target)throw Error('代理实例不存在，请刷新后重试');
 target.enabled=enabled;requireOk(await api.saveConfig(rows));
 const applied=requireOk(await api.fetchOverview());
 const running=(applied.status||[]).find(row=>row.id===id)?.running===true;
 if(enabled&&!running)requireOk(await api.startInstance(id));
 if(!enabled&&running)requireOk(await api.stopInstance(id));
}
