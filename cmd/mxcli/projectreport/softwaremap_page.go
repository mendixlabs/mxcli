// SPDX-License-Identifier: Apache-2.0

package projectreport

// mapCSS adds the interactive page's controls to pageCSS.
const mapCSS = `
.tabs{display:flex;flex-wrap:wrap;gap:4px;margin:0 0 18px;border-bottom:1px solid var(--line)}
.tabs button{background:none;border:0;border-bottom:2px solid transparent;color:var(--muted);padding:8px 10px;font:inherit;cursor:pointer}
.tabs button.on{color:var(--fg);border-color:var(--accent)}
.bar{display:flex;flex-wrap:wrap;gap:12px;align-items:center;margin:8px 0 12px;font-size:14px}
.bar select,.bar input[type=search],.bar input[type=number]{font:inherit;padding:4px 6px;border:1px solid var(--line);border-radius:6px;background:var(--card);color:var(--fg)}
.bar input[type=search]{min-width:260px}
.cards{display:grid;grid-template-columns:repeat(auto-fill,minmax(250px,1fr));gap:10px}
.card{background:var(--card);border:1px solid var(--line);border-radius:8px;padding:8px 10px;font-size:13px}
.card.ghost{border-style:dashed;opacity:.75}.card h4{margin:0 0 4px;font-size:14px}
.card ul{margin:4px 0 0;padding-left:16px}.card{overflow-wrap:anywhere}.muted{color:var(--muted)}
a.doc{color:var(--accent);text-decoration:none;cursor:pointer}a.doc:hover{text-decoration:underline}
.pill{display:inline-block;border:1px solid var(--line);border-radius:10px;padding:0 7px;margin:1px 2px;font-size:12px;color:var(--muted)}
.detail{background:var(--card);border:1px solid var(--line);border-radius:8px;padding:10px 12px;margin:10px 0}
svg .node rect{stroke:var(--line)}svg .node{cursor:pointer}svg .node:hover rect{stroke:var(--accent)}
svg .edge{fill:none;stroke:var(--muted);opacity:.7}svg .edge.nav{stroke-dasharray:4 3}
.graph{overflow:auto;border:1px solid var(--line);border-radius:8px;background:var(--card);max-height:640px}
`

// mapJS is the page's script. Everything it shows comes from the embedded
// JSON; it makes no requests. Graphs are laid out with a small layered layout
// (longest-path ranks, barycenter ordering) and are always bounded: one module,
// one entry point or one role at a time.
const mapJS = `
(function(){
"use strict";
var P = JSON.parse(document.getElementById('map-data').textContent);
var D = P.data;
function esc(s){return String(s==null?'':s).replace(/[&<>"']/g,function(c){return {'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c];});}
function mod(q){var i=q.indexOf('.');return i>0?q.slice(0,i):'';}
function short(q){var i=q.lastIndexOf('.');return i>=0?q.slice(i+1):q;}
function link(q){return '<a class="doc" data-doc="'+esc(q)+'">'+esc(q)+'</a>';}
function links(xs){return (xs||[]).map(link).join(', ');}
function by(xs,k){var m={};(xs||[]).forEach(function(x){m[x[k]]=x;});return m;}
function table(cols,rows){
  var h='<div class="tw"><table><thead><tr>'+cols.map(function(c){return '<th>'+esc(c)+'</th>';}).join('')+'</tr></thead><tbody>';
  if(!rows.length){h+='<tr><td colspan="'+cols.length+'" class="none">none</td></tr>';}
  rows.forEach(function(r){h+='<tr>'+r.map(function(c){return '<td>'+c+'</td>';}).join('')+'</tr>';});
  return h+'</tbody></table></div>';
}
function h2(t,intro){return '<h2>'+esc(t)+'</h2>'+(intro?'<p class="intro">'+esc(intro)+'</p>':'');}
var flows=by(D.flows,'name'), pages=by(D.pages,'name'), ents=by(D.entities,'name'), index=by(D.index,'name');

// ---- layered layout
function layout(nodes, edges){
  var ids=nodes.map(function(n){return n.id;}), has={}; ids.forEach(function(i){has[i]=1;});
  edges=edges.filter(function(e){return has[e.from]&&has[e.to]&&e.from!==e.to;});
  var out={},inn={}; ids.forEach(function(i){out[i]=[];inn[i]=[];});
  // break cycles: DFS back edges are ignored for ranking
  var state={},keep=[];
  function dfs(v){state[v]=1;edges.forEach(function(e){ if(e.from!==v)return; if(state[e.to]===1)return; keep.push(e); if(!state[e.to])dfs(e.to);});state[v]=2;}
  ids.forEach(function(i){if(!state[i])dfs(i);});
  keep.forEach(function(e){out[e.from].push(e.to);inn[e.to].push(e.from);});
  var rank={};
  function r(v,seen){ if(rank[v]!=null)return rank[v]; seen[v]=1; var m=0; inn[v].forEach(function(u){ if(!seen[u]) m=Math.max(m,r(u,seen)+1);}); delete seen[v]; return rank[v]=m;}
  ids.forEach(function(i){r(i,{});});
  var layers=[]; ids.forEach(function(i){(layers[rank[i]]=layers[rank[i]]||[]).push(i);});
  layers=layers.filter(Boolean); layers.forEach(function(l){l.sort();});
  var pos={}; function setpos(){layers.forEach(function(l){l.forEach(function(v,k){pos[v]=k;});});}
  setpos();
  for(var it=0;it<4;it++){
    for(var li=1;li<layers.length;li++){ layers[li].sort(function(a,b){return bary(a,inn)-bary(b,inn);}); setpos(); }
    for(li=layers.length-2;li>=0;li--){ layers[li].sort(function(a,b){return bary(a,out)-bary(b,out);}); setpos(); }
  }
  function bary(v,adj){var a=adj[v]; if(!a.length)return pos[v]; var s=0; a.forEach(function(u){s+=pos[u];}); return s/a.length;}
  var W={},label={}; nodes.forEach(function(n){label[n.id]=n; W[n.id]=Math.min(320,Math.max(70,6.6*(n.label||n.id).length+18));});
  var gapX=24,gapY=70,H=30,coord={},width=0;
  layers.forEach(function(l,li){ var x=16; l.forEach(function(v){coord[v]={x:x,y:16+li*(H+gapY),w:W[v]}; x+=W[v]+gapX;}); width=Math.max(width,x);});
  var height=16+layers.length*(H+gapY);
  var s='<svg xmlns="http://www.w3.org/2000/svg" width="'+width+'" height="'+height+'"><defs><marker id="ah" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0,0L10,5L0,10z" style="fill:var(--muted)"/></marker></defs>';
  edges.forEach(function(e){
    var a=coord[e.from],b=coord[e.to]; var x1=a.x+a.w/2,y1=a.y+H,x2=b.x+b.w/2,y2=b.y;
    if(b.y<=a.y){y1=a.y;y2=b.y+H;}
    var my=(y1+y2)/2, sw=e.weight?Math.min(5,1+Math.log(1+e.weight)):1.2;
    s+='<path class="edge'+(e.nav?' nav':'')+'" d="M'+x1+','+y1+' C'+x1+','+my+' '+x2+','+my+' '+x2+','+y2+'" stroke-width="'+sw.toFixed(1)+'" marker-end="url(#ah)"><title>'+esc(e.title||(e.from+' → '+e.to))+'</title></path>';
  });
  nodes.forEach(function(n){
    var c=coord[n.id]; var fill=n.fill||'var(--okbg)';
    s+='<g class="node" data-id="'+esc(n.id)+'"><title>'+esc(n.title||n.id)+'</title><rect x="'+c.x+'" y="'+c.y+'" width="'+c.w+'" height="'+H+'" rx="6" style="fill:'+fill+(n.dashed?';stroke-dasharray:4 3':'')+'"/>';
    s+='<text x="'+(c.x+c.w/2)+'" y="'+(c.y+19)+'" text-anchor="middle">'+esc(fit(n.label||n.id,c.w))+'</text></g>';
  });
  return '<div class="graph">'+s+'</svg></div>';
}
function fit(t,w){var n=Math.floor((w-14)/6.6); return t.length<=n?t:t.slice(0,n-1)+'…';}
function wire(el,onNode){
  el.querySelectorAll('.node').forEach(function(g){g.addEventListener('click',function(){onNode(g.getAttribute('data-id'));});});
}

// ---- views
var V={};
V.landscape=function(){
  var h=h2('User roles','Where each role starts, and how many of the pages it may open a click path reaches.');
  h+=table(['Role','Starts at','Pages reached','May open, no path reaches'],D.roles.map(function(r){
    return [esc(r.name)+(r.anonymous?' <span class="pill">anonymous</span>':''),links(r.starts),(r.reachable||[]).length+' of '+r.allowed,
      (r.unreached||[]).length?'<details><summary>'+r.unreached.length+'</summary>'+links(r.unreached)+'</details>':'0'];}));
  h+=h2('Systems around the app');
  h+=table(['Kind','Name','Detail'],(D.systems||[]).map(function(s){return [esc(s.kind),link(s.name),esc(s.detail)];}));
  h+=h2('Runs without a user','Startup, schedules, task queues, workflows, entity events and flows named in constants.');
  h+=table(['Started by','Source','Flow'],(D.background||[]).map(function(t){return [esc(t.kind),esc(t.source),link(t.flow)];}));
  return h;
};
V.features=function(arg){
  function arr(x){return x||[];}
  var fs=arr(D.features).map(function(f){['requirements','modules','pages','flows','core','entities','other','entryPoints','roles','sharesWith'].forEach(function(k){f[k]=arr(f[k]);});f.status=f.status||{};return f;});
  var u=D.unowned||{}; u={pages:arr(u.pages),entryPoints:arr(u.entryPoints),entities:arr(u.entities)};
  var h='<div class="bar">Feature <select id="ft-sel"><option value="">All features</option>'+fs.map(function(f){return '<option'+(f.name===arg?' selected':'')+'>'+esc(f.name)+'</option>';}).join('')+'</select></div>';
  var f=fs.filter(function(x){return x.name===arg;})[0];
  function status(st){return ['built','partial','changed','not built','not traceable'].filter(function(k){return st[k];}).map(function(k){return '<span class="pill">'+st[k]+' '+esc(k)+'</span>';}).join('');}
  if(!f){
    h+=h2('Features','Each feature is a slice of the project brain\'s plan; what belongs to it is derived from its requirements\' anchors. Pick one to see its pages, flows and data.');
    h+='<div class="cards">'+fs.map(function(f){return '<div class="card"><h4><a class="doc" data-feature="'+esc(f.name)+'">'+esc(f.name)+'</a>'+(f.milestone?' <span class="pill">'+esc(f.milestone)+'</span>':'')+'</h4>'+
      '<div>'+status(f.status)+'</div><div class="muted">'+f.requirements.length+' requirement(s) · '+f.pages.length+' page(s) · '+f.flows.length+' flow(s) · '+f.entities.length+' entit'+(f.entities.length===1?'y':'ies')+'</div>'+
      '<div class="muted">'+esc(f.modules.join(', '))+'</div>'+(f.sharesWith&&f.sharesWith.length?'<div class="muted">shares with '+esc(f.sharesWith.join(', '))+'</div>':'')+'</div>';}).join('')+'</div>';
    h+=h2('Reached by no feature','The part of the app no requirement leads to: specified nowhere, or reached only through navigation the plan does not anchor.');
    h+=table(['Kind','Count','Documents'],[['pages',u.pages.length,links(u.pages)],['entry points',u.entryPoints.length,links(u.entryPoints)],['persistent entities',u.entities.length,links(u.entities)]].map(function(r){return [esc(r[0]),r[1],r[2]];}));
    return h;
  }
  h+='<p>'+(f.milestone?'Milestone <b>'+esc(f.milestone)+'</b>. ':'')+'Modules: '+esc(f.modules.join(', ')||'none')+'.'+(f.roles.length?' Roles that may open its pages: '+esc(f.roles.join(', '))+'.':'')+
     (f.sharesWith&&f.sharesWith.length?' Shares pages, flows or data with '+f.sharesWith.map(function(s){return '<a class="doc" data-feature="'+esc(s)+'">'+esc(s)+'</a>';}).join(', ')+'.':'')+'</p>';
  h+=h2('Requirements');
  h+=table(['Id','Requirement','Status','Milestone'],f.requirements.map(function(r){return ['<code>'+esc(r.id)+'</code>',esc(r.title),esc(r.status),esc(r.milestone||'')];}));
  var keep={},nodes=[],edges=[];
  f.pages.forEach(function(p){keep[p]=1;nodes.push({id:p,label:p,fill:'var(--warnbg)'});});
  var core={}; f.core.forEach(function(n){core[n]=1;});
  var shown=f.core.concat(f.flows.filter(function(n){return !core[n];})).slice(0,60);
  shown.forEach(function(n){keep[n]=1;var fl=flows[n];nodes.push({id:n,label:(core[n]?'★ ':'')+n,fill:fl&&fl.nanoflow?'var(--infobg)':'var(--okbg)'});});
  f.entities.forEach(function(e){keep[e]=1;nodes.push({id:e,label:e,fill:'var(--card)',dashed:1});});
  f.flows.forEach(function(n){var fl=flows[n]; if(!fl||!keep[n])return;
    (fl.calls||[]).forEach(function(c){if(keep[c])edges.push({from:n,to:c,title:'calls'});});
    (fl.pages||[]).forEach(function(p){if(keep[p])edges.push({from:n,to:p,nav:1,title:'opens'});});});
  f.pages.forEach(function(p){var pg=pages[p]; (pg&&pg.flows||[]).forEach(function(n){if(keep[n])edges.push({from:p,to:n,title:'button runs'});});});
  f.entities.forEach(function(e){(ents[e]&&ents[e].writtenBy||[]).forEach(function(n){if(keep[n])edges.push({from:n,to:e,title:'writes'});});});
  h+=h2('Pages, flows and data','Amber: pages; green: microflows; blue: nanoflows; dashed: entities. Arrows: a button runs a flow, a flow calls, opens or writes. ★ marks the '+f.core.length+' flow(s) the requirements are anchored on or whose footprint they are; the rest is their reach'+(f.flows.length>60?' (60 of '+f.flows.length+' shown)':'')+'.');
  h+=nodes.length?layout(nodes,edges):'<p>Nothing in the app is anchored yet.</p>';
  h+=h2('Entry points','Flows in the feature\'s reach that something outside it starts: a schedule, a service, the menu, or a button on a page that is not the feature\'s own.');
  h+=table(['Started by','Flow','From'],(f.entryPoints||[]).map(function(e){return [esc(e.kind),link(e.flow),esc(e.source)];}));
  if((f.other||[]).length)h+=h2('Other anchors','Anchors that are not pages, flows or entities: services, workflows, roles, enumerations.')+'<p>'+links(f.other)+'</p>';
  return h;
};
V.features.after=function(root){
  root.querySelector('#ft-sel').addEventListener('change',function(e){go('features',e.target.value);});
  root.querySelectorAll('a[data-feature]').forEach(function(a){a.addEventListener('click',function(){go('features',a.getAttribute('data-feature'));});});
  root.querySelectorAll('.node').forEach(function(g){g.addEventListener('click',function(){openDoc(g.getAttribute('data-id'));});});
};
V.modules=function(){
  var h=h2('Module map','Arrows point from a module to what it uses, thicker for more references. Screen links (dashed) and the hub are hidden unless switched on.');
  h+='<div class="bar"><label><input type="checkbox" id="m-nav"> screen links</label>'+(D.hub?'<label><input type="checkbox" id="m-hub"> hub ('+esc(D.hub)+')</label>':'')+
     '<label>hide links under <input type="number" id="m-min" value="1" min="1" style="width:60px"> references</label></div><div id="m-graph"></div>';
  h+=h2('Modules');
  h+=table(['Module','Purpose','Entities','Flows','Pages','Entry points'],D.modules.map(function(m){
    var eps=P.entryKinds.filter(function(k){return m.entryPoints[k];}).map(function(k){return '<span class="pill">'+esc(k)+' '+m.entryPoints[k]+'</span>';}).join('');
    return ['<a class="doc" data-mod="'+esc(m.name)+'">'+esc(m.name)+'</a>'+(m.integration?' <span class="pill">integration</span>':''),esc(m.purpose||''),
      m.counts.entity||0,(m.counts.microflow||0)+(m.counts.nanoflow||0),m.counts.page||0,eps];}));
  return h;
};
V.modules.after=function(root){
  function draw(){
    var nav=root.querySelector('#m-nav').checked, hub=root.querySelector('#m-hub'), showHub=hub?hub.checked:true;
    var min=+root.querySelector('#m-min').value||1;
    var nodes=D.modules.filter(function(m){return showHub||m.name!==D.hub;}).map(function(m){return {id:m.name,title:m.purpose||m.name,fill:m.integration?'var(--infobg)':'var(--okbg)'};});
    var edges=[]; (D.moduleEdges||[]).forEach(function(e){
      if(e.domain>=min) edges.push({from:e.from,to:e.to,weight:e.domain,title:e.from+' → '+e.to+': '+e.domain+' references'});
      else if(nav&&e.navigation>=min) edges.push({from:e.from,to:e.to,nav:1,title:e.from+' → '+e.to+': '+e.navigation+' screen links'});
      if(nav&&e.domain>=min&&e.navigation>=min) edges.push({from:e.from,to:e.to,nav:1,title:e.navigation+' screen links'});
    });
    var g=root.querySelector('#m-graph'); g.innerHTML=layout(nodes,edges); wire(g,function(id){go('module',id);});
  }
  root.querySelectorAll('#m-nav,#m-hub,#m-min').forEach(function(el){el.addEventListener('change',draw);});
  draw();
};
V.module=function(arg){
  var m=arg||(D.modules[0]&&D.modules[0].name)||'';
  var h='<div class="bar">Module <select id="mod-sel">'+D.modules.map(function(x){return '<option'+(x.name===m?' selected':'')+'>'+esc(x.name)+'</option>';}).join('')+'</select></div>';
  var mm=D.modules.filter(function(x){return x.name===m;})[0]; if(!mm)return h;
  if(mm.purpose)h+='<p>'+esc(mm.purpose)+'</p>';
  // data model
  var own=D.entities.filter(function(e){return e.module===m;}), ghosts={};
  var assocs=(D.associations||[]).filter(function(a){return mod(a.from)===m||mod(a.to)===m;});
  assocs.forEach(function(a){[a.from,a.to].forEach(function(e){if(mod(e)!==m)ghosts[e]=1;});});
  h+=h2('Data model','Entities of this module; dashed boxes are other modules\' entities it is associated with.');
  var nodes=own.map(function(e){return {id:e.name,label:short(e.name),title:e.attributes.join('\n'),fill:e.persistent?'var(--okbg)':'var(--card)'};})
    .concat(Object.keys(ghosts).map(function(g){return {id:g,label:g,dashed:1,fill:'var(--card)'};}));
  var edges=assocs.map(function(a){return {from:a.from,to:a.to,title:a.name+' ('+a.type+')'};});
  own.forEach(function(e){if(e.generalization&&(ents[e.generalization]||ghosts[e.generalization]))edges.push({from:e.name,to:e.generalization,nav:1,title:'generalizes'});});
  if(nodes.length) h+=layout(nodes,edges);
  h+='<div class="cards">'+own.map(function(e){return '<div class="card"><h4>'+link(e.name)+(e.persistent?'':' <span class="pill">non-persistent</span>')+'</h4>'+
     (e.generalization?'<div class="muted">extends '+link(e.generalization)+'</div>':'')+'<ul>'+e.attributes.map(function(a){return '<li>'+esc(a)+'</li>';}).join('')+'</ul>'+
     ((e.writtenBy||[]).length?'<div class="muted">written by '+links(e.writtenBy)+'</div>':'')+'</div>';}).join('')+'</div>';
  // entry points
  var mf=D.flows.filter(function(f){return f.module===m;});
  var groups={}; mf.forEach(function(f){(f.triggers||[]).forEach(function(t){(groups[t.kind]=groups[t.kind]||[]).push([f.name,t.source]);});});
  h+=h2('Entry points','Grouped by what starts them. Pick one to see its call closure across modules.');
  h+=table(['Started by','Flow','From'],P.entryKinds.filter(function(k){return groups[k];}).reduce(function(acc,k){
     groups[k].forEach(function(x,i){acc.push([i?'':esc(k),'<a class="doc" data-flow="'+esc(x[0])+'">'+esc(x[0])+'</a>',esc(x[1])]);}); return acc;},[]));
  h+='<div class="bar">Flow view <select id="flow-sel"><option value="">—</option>'+mf.map(function(f){return '<option'+((f.triggers||[]).length?' style="font-weight:600"':'')+'>'+esc(f.name)+'</option>';}).join('')+'</select></div><div id="flow-graph"></div>';
  // pages and java actions
  var pg=D.pages.filter(function(p){return p.module===m;});
  h+=h2('Pages');
  h+=table(['Page','Title','Module roles that may open it','Buttons run'],pg.map(function(p){return [link(p.name)+(p.popup?' <span class="pill">pop-up</span>':''),esc(p.title),esc((p.roles||[]).join(', ')),links(p.flows)];}));
  var ja=(D.javaActions||[]).filter(function(j){return j.module===m;});
  if(ja.length){h+=h2('Java actions')+table(['Java action','Does'],ja.map(function(j){return [link(j.name),esc(j.note)];}));}
  return h;
};
V.module.after=function(root,arg){
  root.querySelector('#mod-sel').addEventListener('change',function(e){go('module',e.target.value);});
  var fs=root.querySelector('#flow-sel');
  function draw(name){
    var g=root.querySelector('#flow-graph'); if(!name){g.innerHTML='';return;}
    var seen={},q=[name],nodes=[],edges=[];
    while(q.length&&nodes.length<80){var n=q.shift(); if(seen[n])continue; seen[n]=1; var f=flows[n];
      nodes.push({id:n,label:mod(n)===mod(name)?short(n):n,fill:f?(f.nanoflow?'var(--infobg)':'var(--okbg)'):'var(--card)',dashed:!f});
      if(f){(f.calls||[]).forEach(function(c){edges.push({from:n,to:c});q.push(c);});
        (f.pages||[]).forEach(function(p){edges.push({from:n,to:p,nav:1,title:'opens page'}); if(!seen[p]){seen[p]=1;nodes.push({id:p,label:p,fill:'var(--warnbg)'});}});}}
    g.innerHTML='<p class="muted">Call closure of '+esc(name)+': '+nodes.length+' node(s)'+(q.length?' (cut at 80)':'')+'. Amber: pages it opens; dashed: outside the app.</p>'+layout(nodes,edges);
    wire(g,function(id){ if(flows[id]) {fs.value=id; draw(id);} else openDoc(id); });
  }
  fs.addEventListener('change',function(){draw(fs.value);});
  root.querySelectorAll('a[data-flow]').forEach(function(a){a.addEventListener('click',function(){fs.value=a.getAttribute('data-flow');draw(fs.value);fs.scrollIntoView();});});
  if(arg&&flows[arg]){fs.value=arg;draw(arg);}
};
V.pageflows=function(arg){
  var r=arg||(D.roles[0]&&D.roles[0].name)||'';
  var h='<div class="bar">Role <select id="role-sel">'+D.roles.map(function(x){return '<option'+(x.name===r?' selected':'')+'>'+esc(x.name)+'</option>';}).join('')+'</select></div>';
  var role=D.roles.filter(function(x){return x.name===r;})[0]; if(!role)return h+'<p>No user roles.</p>';
  h+='<p>Starts at '+links(role.starts)+'. Reaches '+(role.reachable||[]).length+' of the '+role.allowed+' pages it may open.</p>';
  var reach={}; (role.reachable||[]).forEach(function(p){reach[p]=1;});
  var nodes=(role.reachable||[]).slice(0,70).map(function(p){var pg=pages[p];return {id:p,label:p,fill:(role.starts||[]).indexOf(p)>=0?'var(--okbg)':(pg&&pg.popup?'var(--infobg)':'var(--card)')};});
  var keep={}; nodes.forEach(function(n){keep[n.id]=1;});
  var edges=(D.pageEdges||[]).filter(function(e){return keep[e.to]&&(keep[e.from]||flows[e.from]);}).map(function(e){
    if(keep[e.from])return {from:e.from,to:e.to,title:e.kind};
    // flow opens page: draw from the pages whose buttons run that flow
    return null;}).filter(Boolean);
  D.pages.forEach(function(p){ if(!keep[p.name])return; (p.flows||[]).forEach(function(f){ (flows[f]&&flows[f].pages||[]).forEach(function(t){ if(keep[t]&&t!==p.name) edges.push({from:p.name,to:t,nav:1,title:'button runs '+f+', which opens '+t}); }); }); });
  h+=h2('Click paths','Green: where the role starts; blue: pop-ups; dashed arrows: a button runs a flow that opens the page.')+(nodes.length?layout(nodes,edges):'<p>No page is reached.</p>');
  if((role.reachable||[]).length>70)h+='<p class="muted">Showing 70 of '+role.reachable.length+' pages.</p>';
  h+=h2('May open, but no click path reaches','Workflow task pages and links from notifications are reached this way on purpose; anything else may be dead.')+'<p>'+(links(role.unreached)||'None.')+'</p>';
  return h;
};
V.pageflows.after=function(root){
  root.querySelector('#role-sel').addEventListener('change',function(e){go('pageflows',e.target.value);});
  root.querySelectorAll('.node').forEach(function(g){g.addEventListener('click',function(){openDoc(g.getAttribute('data-id'));});});
};
V.data=function(){
  var x=(D.associations||[]).filter(function(a){return mod(a.from)!==mod(a.to);});
  x.sort(function(a,b){var pa=mod(a.from)+'→'+mod(a.to),pb=mod(b.from)+'→'+mod(b.to);return pa<pb?-1:pa>pb?1:(a.name<b.name?-1:1);});
  var last='';
  return h2('Data across modules','Every association crossing a module boundary, by module pair.')+
    table(['Modules','Association','From','To','Type'],x.map(function(a){var p=mod(a.from)+' → '+mod(a.to);var c=p===last?'':esc(p);last=p;return [c,link(a.name),link(a.from),link(a.to),esc(a.type)];}));
};
V.conventions=function(){
  var mods=D.modules.map(function(m){return m.name;});
  var h=h2('Naming prefixes','How flows are named, and where.');
  h+=table(['Prefix','Flows'].concat(mods),(D.prefixes||[]).map(function(p){return [esc(p.prefix)+'_',p.total].concat(mods.map(function(m){return p.byModule[m]||'';}));}));
  h+=h2('Shared flows','Flows called from three or more modules.');
  h+=table(['Flow','Called from'],(D.sharedFlows||[]).map(function(s){return [link(s.flow),esc(s.callerModules.join(', '))];}));
  h+=h2('Entry points × module');
  h+=table(['Module'].concat(P.entryKinds),D.modules.map(function(m){return [esc(m.name)].concat(P.entryKinds.map(function(k){return m.entryPoints[k]||'';}));}));
  return h;
};
V.apis=function(){
  var h=h2('Published REST operations','Each operation, the microflow it runs, and the size of that microflow\'s call closure.');
  h+=table(['Service','Operation','Microflow','Closure'],(D.published||[]).map(function(o){return [link(o.service),esc(o.method+' '+o.path),'<a class="doc" data-flowmod="'+esc(o.microflow)+'">'+esc(o.microflow)+'</a>',o.closure];}));
  h+=h2('Integration modules','Integration modules and the parts of them other modules call.');
  var integ=D.modules.filter(function(m){return m.integration;}).map(function(m){
    var used=D.flows.filter(function(f){return f.module===m.name;}).filter(function(f){return D.flows.some(function(g){return g.module!==m.name&&(g.calls||[]).indexOf(f.name)>=0;});}).map(function(f){return f.name;});
    return [esc(m.name),links(used)];});
  h+=table(['Module','Called from other modules'],integ);
  h+=h2('Consumed services, import mappings and HTTP calls');
  h+=table(['Document','What'],(D.consumed||[]).map(function(c){return [link(c.name),esc(c.note)];}));
  h+=table(['Flow','HTTP call'],(D.httpCalls||[]).map(function(c){return [link(c.from),esc(c.to)];}));
  h+=h2('Background work');
  h+=table(['Started by','Source','Flow'],(D.background||[]).map(function(t){return [esc(t.kind),esc(t.source),link(t.flow)];}));
  h+=table(['Task queue','Runs','Put there by'],(D.queues||[]).map(function(x){return [link(x.name),links(x.runs),links(x.callers)];}));
  h+=h2('Java actions')+table(['Java action','Does'],(D.javaActions||[]).map(function(j){return [link(j.name),esc(j.note)];}));
  h+=h2('Constants')+table(['Constant','Type','Default','Names a flow'],(D.constants||[]).map(function(c){return [link(c.name),esc(c.type),esc(c.default),c.namesFlow?link(c.namesFlow):''];}));
  return h;
};
V.index=function(arg){
  var h='<div class="bar"><input type="search" id="ix-q" placeholder="Search '+(D.index||[]).length+' documents" value="'+esc(arg||'')+'"> <span class="muted" id="ix-n"></span></div><div id="ix-detail"></div><div id="ix-list"></div>';
  return h;
};
V.index.after=function(root,arg){
  var q=root.querySelector('#ix-q');
  function list(){
    var s=q.value.toLowerCase(), xs=(D.index||[]).filter(function(d){return !s||d.name.toLowerCase().indexOf(s)>=0||d.type.toLowerCase()===s;});
    root.querySelector('#ix-n').textContent=xs.length+' match'+(xs.length===1?'':'es');
    root.querySelector('#ix-list').innerHTML=table(['Document','Type','Uses','Used by'],xs.slice(0,400).map(function(d){return [link(d.name),esc(d.type),(d.uses||[]).length,(d.usedBy||[]).length];}))+(xs.length>400?'<p class="muted">First 400 shown; narrow the search.</p>':'');
    bindDocs(root.querySelector('#ix-list'));
  }
  q.addEventListener('input',list); list();
  if(arg&&index[arg])showDoc(arg);
};
function showDoc(name){
  var root=document.getElementById('ix-detail'); if(!root)return;
  var d=index[name]; if(!d){root.innerHTML='<div class="detail">'+esc(name)+' is not an app document (Marketplace, System or a member).</div>';return;}
  var extra='';
  var e=ents[name]; if(e) extra+='<p>'+esc(e.attributes.join(' · '))+'</p>';
  var f=flows[name]; if(f&&(f.triggers||[]).length) extra+='<p>Started by: '+f.triggers.map(function(t){return esc(t.kind+' ('+t.source+')');}).join(', ')+'</p>';
  root.innerHTML='<div class="detail"><h3 style="margin:0">'+esc(d.name)+' <span class="pill">'+esc(d.type)+'</span> <a class="doc" data-mod="'+esc(d.module)+'">'+esc(d.module)+'</a></h3>'+extra+
    '<p><b>Uses</b> '+(links(d.uses)||'nothing')+'</p><p><b>Used by</b> '+(links(d.usedBy)||'nothing')+'</p></div>';
  bindDocs(root); root.scrollIntoView({block:'nearest'});
}
function openDoc(name){ if(cur!=='index') go('index',''); showDoc(name); }

// ---- shell
var tabs=[['landscape','Landscape']].concat((D.features||[]).length?[['features','Features']]:[]).concat([['modules','Module map'],['module','Inside a module'],['pageflows','Page flows'],['data','Data across modules'],['conventions','Conventions'],['apis','APIs & background'],['index','Index'],['about','How this was made']]);
V.about=function(){
  return h2('How this was measured')+'<ul>'+P.method.map(function(m){return '<li>'+esc(m)+'</li>';}).join('')+'</ul>'+
    h2('What this does not cover')+'<ul>'+P.limits.map(function(m){return '<li>'+esc(m)+'</li>';}).join('')+'</ul>';
};
var cur='';
function bindDocs(root){
  root.querySelectorAll('a[data-doc]').forEach(function(a){a.addEventListener('click',function(){openDoc(a.getAttribute('data-doc'));});});
  root.querySelectorAll('a[data-mod]').forEach(function(a){a.addEventListener('click',function(){go('module',a.getAttribute('data-mod'));});});
  root.querySelectorAll('a[data-flowmod]').forEach(function(a){a.addEventListener('click',function(){var n=a.getAttribute('data-flowmod');go('module',mod(n),n);});});
}
function go(view,arg,arg2){
  cur=view;
  document.querySelectorAll('#tabs button').forEach(function(b){b.className=b.getAttribute('data-v')===view?'on':'';});
  var root=document.getElementById('view');
  var pre=(view==='landscape'&&P.summary?'<p>'+esc(P.summary)+'</p>':'')+((view==='landscape'&&P.warnings)?P.warnings.map(function(w){return '<div class="callout warn">'+esc(w)+'</div>';}).join(''):'');
  root.innerHTML=pre+V[view](arg);
  bindDocs(root);
  if(V[view].after)V[view].after(root,arg2||arg);
  try{history.replaceState(null,'','#'+view+(arg?'/'+encodeURIComponent(arg):''));}catch(e){}
}
document.getElementById('sub').textContent=P.project+' · generated '+P.generated+(P.mxcli?' · mxcli '+P.mxcli:'')+' · '+D.modules.length+' app modules, '+(D.index||[]).length+' documents';
document.getElementById('tabs').innerHTML=tabs.map(function(t){return '<button data-v="'+t[0]+'">'+t[1]+'</button>';}).join('');
document.querySelectorAll('#tabs button').forEach(function(b){b.addEventListener('click',function(){go(b.getAttribute('data-v'));});});
var h=(location.hash||'').slice(1).split('/'); go(V[h[0]]?h[0]:'landscape',h[1]?decodeURIComponent(h[1]):'');
})();
`
