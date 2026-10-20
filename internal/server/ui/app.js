// SPDX-License-Identifier: GPL-2.0-only
'use strict';
const content=document.querySelector('#content'),heading=document.querySelector('#heading'),message=document.querySelector('#message');let csrf='';let feedURLs={};let savedIGDBSecret=false;
const el=(tag,text,cls)=>{const n=document.createElement(tag);if(text!==undefined)n.textContent=String(text);if(cls)n.className=cls;return n};
function notify(text,error=false){message.textContent=text;message.className=error?'error':'success'}
let pendingRequests=0,requestTimer;
const requestStatus=document.querySelector('#request-status');
function requestStarted(){
 pendingRequests++;document.querySelector('main').setAttribute('aria-busy','true');
 if(pendingRequests===1)requestTimer=setTimeout(()=>{requestStatus.hidden=false},150);
}
function requestFinished(){
 pendingRequests=Math.max(0,pendingRequests-1);
 if(!pendingRequests){clearTimeout(requestTimer);requestStatus.hidden=true;document.querySelector('main').setAttribute('aria-busy','false')}
}
async function api(path,method='GET',body,quiet=false){
 if(!quiet)requestStarted();
 try{
  const options={method,headers:{'X-CSRF-Token':csrf}};
  if(body!==undefined){options.headers['Content-Type']='application/json';options.body=JSON.stringify(body)}
  let r;try{r=await fetch('/api/v1/'+path,options)}catch{throw Error('Cannot reach the server. Check your connection and that the server is running, then try again.')}
  let b;try{b=await r.json()}catch{throw Error(`The server returned an unexpected response (${r.status}). Check the server logs or reverse proxy, then try again.`)}
  if(!r.ok){const defaults={401:'Please sign in again. Your session may have expired.',403:'This action could not be verified. Reload the page and try again.',409:'A scan is already running. Wait for it to finish or cancel it first.',429:'Too many sign-in attempts. Wait 15 minutes, then try again.'};throw Error((['login required','CSRF token required'].includes(b.error)?defaults[r.status]:b.error)||defaults[r.status]||`Request failed (${r.status}). Please try again.`)}
  return b;
 }finally{if(!quiet)requestFinished()}
}

function button(text,fn,cls){
 const b=el('button',text,cls);b.type='button';
 b.addEventListener('click',async()=>{b.disabled=true;try{await fn()}catch(e){notify(e.message,true)}finally{b.disabled=false}});return b;
}
function field(parent,text,value,type='text',help=''){
 const l=el('label',text),i=el('input');i.type=type;
 if(type==='checkbox')i.checked=Boolean(value);else i.value=value??'';
 l.append(i);if(help)l.append(el('small',help,'hint'));parent.append(l);return i;
}
function invalid(input,text){
 for(const old of input.parentElement.querySelectorAll('.field-error'))old.remove();input.setAttribute('aria-invalid','true');const hint=el('span',text,'field-error');input.parentElement.append(hint);input.focus();
 input.addEventListener('input',()=>{input.removeAttribute('aria-invalid');hint.remove();message.textContent=''},{once:true});throw Error(text);
}
function scanProgress(){
 const box=el('div',undefined,'scan-progress'),top=el('div',undefined,'scan-top'),title=el('span',undefined,'scan-title'),count=el('span',undefined,'scan-count'),bar=el('progress'),note=el('p',undefined,'scan-current'),current=el('p',undefined,'scan-current');
 bar.setAttribute('aria-label','Scan progress');top.append(title,count);box.append(top,bar,note,current);
 return {box,update(j,packages){
  const phase=j.phase|| (j.running?'discovering':'idle');
  const titles={idle:'Ready to scan',discovering:'Finding package files',reading:'Reading packages',publishing:'Saving the library',metadata:'Fetching game details',complete:'Scan complete',cancelled:'Scan stopped',failed:'Scan could not finish'};
  title.textContent=titles[phase]||'Scanning';
  let done=0,total=0;
  if(phase==='metadata'){done=j.metadata_done||0;total=j.metadata_total||0;count.textContent=total?`${done} of ${total} titles`:''}
  else if(phase==='reading'){done=j.visited||0;total=j.total||0;count.textContent=`${done}${total?' of '+total:''} packages read`}
  else count.textContent=`${packages||0} packages in library`;
  if(!j.running){bar.max=1;bar.value=phase==='failed'?0:1}
  else if(total){bar.max=total;bar.value=Math.min(done,total)}else bar.removeAttribute('value');
  note.textContent=phase==='metadata'?'Packages are available now. Optional details and screenshots are being added; you can keep browsing.':phase==='discovering'?'Looking through your enabled folders, including subfolders.':phase==='reading'?'Reading package headers and covers. Game files are not copied or hashed.':phase==='cancelled'?'Packages already published remain available. You can start another scan when ready.':phase==='idle'?'Add a folder, then choose Scan now.':phase==='complete'?(j.errors?.length||j.warnings?'Finished with warnings. See Administration for details.':'Your library is up to date.'):phase==='failed'?'Check the errors below or in Administration. Existing published packages remain available.':'Updating catalogs and the local cache.';
  current.textContent=j.current||'';
 }};
}
function area(parent,label,value){const l=el('label',label),i=el('textarea');i.value=value;l.append(i);parent.append(l);return i}
async function copy(value,source){
 // The modern API is restricted to secure contexts; LAN HTTP needs the
 // synchronous copy-command fallback inside the user's button activation.
 if(window.isSecureContext&&navigator.clipboard?.writeText){
  try{await navigator.clipboard.writeText(value);notify('Link copied');return}catch{}
 }
 const active=document.activeElement,buffer=el('textarea');buffer.value=value;buffer.readOnly=true;buffer.className='clipboard-buffer';buffer.setAttribute('aria-label','Clipboard buffer');
 (document.querySelector('dialog[open]')||document.body).append(buffer);
 let copied=false;
 try{buffer.focus({preventScroll:true});buffer.select();buffer.setSelectionRange(0,value.length);copied=document.execCommand('copy')}catch{}
 finally{buffer.remove();if(active?.isConnected)active.focus({preventScroll:true})}
 if(copied){notify('Link copied');return}
 // When browser policy blocks both methods, select the existing URL instead
 // of opening another modal or claiming that copying succeeded.
 const target=source||el('input');target.value=value;target.readOnly=true;
 if(!source){target.setAttribute('aria-label','Link to copy');(document.querySelector('#details[open] #detail-content')||content).append(target)}
 target.focus();target.select();target.setSelectionRange(0,value.length);
 notify('Your browser blocked automatic copying. The link is selected; press Ctrl+C or ⌘C to copy it.');
}

function feedList(cats){
 const d=el('div',undefined,'feeds');d.append(el('h2','Game feeds'),el('p','Copy the feed URLs you want to use in FPKGi.','muted'));
 for(const cat of Object.keys(cats).sort()){
  const row=el('div',undefined,'feed'),i=el('input');i.readOnly=true;i.value=feedURLs[cat]||location.origin+'/feeds/'+encodeURIComponent(cat);i.setAttribute('aria-label',cat+' feed URL');row.append(el('span',cat),i,button('Copy',()=>copy(i.value,i)));d.append(row);
 }
 return d;
}
function safeURL(s){try{const u=new URL(s,location.origin);if(['http:','https:'].includes(u.protocol))return u.href}catch{}return ''}
function showDetails(game){const panel=document.querySelector('#detail-content');panel.replaceChildren(el('h2',game.name),el('p',`${game.title_id||'Homebrew'} · ${game.package_type} · ${game.version} · ${game.min_fw||'Unknown'} firmware`));const metadata=game.igdb||{};if(metadata.summary)panel.append(el('p',metadata.summary));if(metadata.storyline)panel.append(el('p',metadata.storyline));for(const key of ['genres','themes','developers','publishers','game_modes'])if(metadata[key]?.length)panel.append(el('p',`${key.replaceAll('_',' ')}: ${metadata[key].join(', ')}`));const shots=el('div',undefined,'shots');for(const shot of metadata.media?.screenshots||[]){const src=safeURL(shot.url);if(src){const img=el('img');img.src=src;img.loading='lazy';img.alt='Game screenshot';shots.append(img)}}panel.append(shots);if(metadata.relationships){panel.append(el('h3','Related games'));for(const [key,items] of Object.entries(metadata.relationships)){if(Array.isArray(items)&&items.length)panel.append(el('p',`${key.replaceAll('_',' ')}: ${items.map(i=>i.name).join(', ')}`))}}const technical=el('details');technical.append(el('summary','All metadata'),el('pre',JSON.stringify(game,null,2)));panel.append(technical,button('Copy package URL',()=>copy(game.package_url)));document.querySelector('#details').showModal()}
document.querySelector('#close-details').onclick=()=>document.querySelector('#details').close();
async function library(){
 document.body.classList.add('library');let cats=await api('library');feedURLs=await api('feeds');const all=[];
 function unpack(){all.length=0;for(const [category,cat]of Object.entries(cats))for(const [url,g]of Object.entries(cat.DATA||{}))all.push({...g,category,package_url:url})}unpack();
 const scan=scanProgress();scan.box.hidden=true;content.append(scan.box);
 const tools=el('div',undefined,'toolbar'),search=el('input'),category=el('select'),type=el('select');search.placeholder='Search games or title IDs';search.setAttribute('aria-label','Search library');
 for(const [select,values,label]of [[category,Object.keys(cats).sort(),'All categories'],[type,['game','update','dlc','unknown'],'All package types']]){select.setAttribute('aria-label',label);const o=el('option',label);o.value='';select.append(o);for(const v of values){const option=el('option',v);option.value=v;select.append(option)}}
 tools.append(search,category,type);const count=el('p',undefined,'muted'),grid=el('div',undefined,'grid'),feeds=feedList(cats);content.append(feeds,tools,count,grid);
 function draw(){grid.replaceChildren();const list=all.filter(g=>(!category.value||g.category===category.value)&&(!type.value||g.package_type===type.value)&&`${g.name} ${g.title_id} ${g.content_id}`.toLowerCase().includes(search.value.toLowerCase()));count.textContent=`${list.length} packages`;
 for(const g of list){const card=button('',()=>showDetails(g),'card'),img=el('img');img.src=safeURL(g.cover_url)||'/fallback.svg';img.loading='lazy';img.alt=g.name;img.onerror=()=>{img.onerror=null;img.src='/fallback.svg'};const text=el('div');text.append(el('span',g.package_type,'badge'),el('h3',g.name),el('small',`${g.title_id||'Homebrew'} · v${g.version}`));card.append(img,text);grid.append(card)}
 if(!all.length){const empty=el('div',undefined,'empty');empty.append(el('h3','No packages published yet'),el('p','If a scan is running, its progress appears above. Otherwise, add your package folder and start a scan in Administration.'));const link=el('a','Open Administration');link.href='/admin';empty.append(link);grid.append(empty)}else if(!list.length)grid.append(el('p','No matches. Try another search or clear the filters.','empty'));
 }for(const i of [search,category,type])i.addEventListener('input',draw);draw();
 let refreshing=false,wasRunning=false;async function refresh(){if(refreshing)return;refreshing=true;try{const j=await api('library/status','GET',undefined,true);scan.box.hidden=!j.running&&!j.warnings&&j.phase!=='failed'&&j.phase!=='cancelled';scan.update(j,j.packages);if(j.packages!==all.length||(wasRunning&&!j.running)){cats=await api('library','GET',undefined,true);unpack();draw()}wasRunning=j.running}catch(e){scan.box.hidden=false;scan.update({phase:'failed'},all.length);notify(e.message,true)}finally{refreshing=false}}
 await refresh();setInterval(refresh,2000);
}
async function login(){heading.textContent='Sign in';if(new URLSearchParams(location.search).has('restart'))notify('Settings saved. Restart the server to apply the new listeners. You can sign in here to finish the first scan.');const form=el('form',undefined,'panel narrow');const user=field(form,'Administrator username','','text'),pass=field(form,'Password','','password');user.autocomplete='username';pass.autocomplete='current-password';const submit=el('button','Sign in','primary');form.append(submit);form.onsubmit=async e=>{e.preventDefault();try{const b=await api('auth/login','POST',{username:user.value,password:pass.value});csrf=b.csrf;if(new URLSearchParams(location.search).has('first')){try{await api('scan','POST',{force:false})}catch(e){if(!e.message.includes('already running'))throw e}}location.href='/admin'}catch(err){notify(err.message,true)}};content.append(form)}
function mappingEditor(parent,c){
 const wrap=el('div');parent.append(wrap);const rows=[];
 function add(m={id:'folder-'+Math.random().toString(36).slice(2,9),path:c.data_dir==='/data'?'/packages':'',category:'games',enabled:true}){
 const box=el('div',undefined,'mapping'),path=field(box,'Package folder',m.path,'text',c.data_dir==='/data'?'Use /packages for the mounted games folder. Other host folders need a Compose mount.':'Choose a folder on the computer running the server. Subfolders are included.');path.placeholder=c.data_dir==='/data'?'/packages':'C:\\PS4Packages or /srv/ps4';
 const controls=el('div',undefined,'row');controls.append(button('Browse folders',()=>browse(path)),button('Remove',()=>{box.remove();rows.splice(rows.indexOf(row),1)}));box.append(controls);
 const grid=el('div',undefined,'form-grid'),cat=field(grid,'Feed category',m.category,'text','Use games, updates, dlc, or your own short name.'),enabled=field(grid,'Include this folder',m.enabled,'checkbox');box.append(grid);
 const advanced=el('details');advanced.append(el('summary','Folder identifier'));const id=field(advanced,'Stable mapping ID',m.id,'text','Generated automatically. Keep it unchanged to reuse cached scans.');box.append(advanced);
 const row={box,id,path,cat,enabled};rows.push(row);wrap.append(box);
 }
 for(const m of c.mappings||[])add(m);parent.append(button('Add folder',()=>add()));return()=>rows.map(r=>({id:r.id.value.trim(),path:r.path.value.trim(),category:r.cat.value.trim(),enabled:r.enabled.checked}));
}
async function browse(input){
 const d=el('dialog'),body=el('div',undefined,'browser'),path=el('input'),error=el('p',undefined,'error');path.value=input.value||'';path.setAttribute('aria-label','Directory path');
 const select=button('Use this folder',()=>{input.value=path.value;d.close();d.remove()},'primary');select.disabled=true;
 const controls=el('div',undefined,'actions');controls.append(button('Cancel',()=>{d.close();d.remove()}),select);
 d.append(el('h2','Choose a server folder'),el('p','These folders belong to the server. In Docker, /packages is the mounted games folder. Host folders must be added to Compose first.','muted'),path,button('Open path',()=>load(path.value)),error,body,controls);document.body.append(d);d.showModal();
 async function load(p){select.disabled=true;error.textContent='';const loading=el('progress');loading.setAttribute('aria-label','Loading folders');body.replaceChildren(loading,el('p','Reading folders from the server…','muted'));try{const b=await api('directories?path='+encodeURIComponent(p));path.value=b.path;body.replaceChildren(button('↑ Parent folder',()=>load(b.parent)));for(const root of b.roots||[])body.append(button(root,()=>load(root)));for(const dir of b.directories)body.append(button(dir.split(/[\\/]/).filter(Boolean).pop()||dir,()=>load(dir)));if(!b.directories.length)body.append(el('p','No subfolders. You can select this folder.','muted'));select.disabled=false}catch(e){body.replaceChildren();error.textContent=e.message}}
 await load(input.value||'');
}
function generalFields(parent,c){
 const pub=field(parent,'Public URL for your PS4',c.public_url,'url','The address your PS4 uses to reach this server, for example http://192.168.1.20:8080. Include the published Docker port.');pub.placeholder='http://192.168.1.20:8080';
 const data=field(parent,'Application data folder',c.data_dir,'text','Stores covers and catalogs. Keep /data in Docker; package files stay in your package folder.');
 const advanced=el('details');advanced.append(el('summary','Advanced network and appearance settings'));
 const grid=el('div',undefined,'form-grid'),listen=field(grid,'Web listener',c.listen,'text','Keep :8080 in Docker. The published host port belongs in Public URL.'),dl=field(grid,'Separate download listener',c.download_listen,'text','Optional. Leave blank to use the web listener.');advanced.append(grid);
 const du=field(advanced,'Download public URL',c.download_url,'url','Only needed for a separate download listener.'),bg=field(advanced,'Background image path',c.background);parent.append(advanced);
 return()=>({listen:listen.value,public_url:pub.value.trim().replace(/\/$/,''),data_dir:data.value.trim(),download_listen:dl.value,download_url:du.value.trim().replace(/\/$/,''),background:bg.value});
}
function credentials(parent,c,required=false){
 const u=field(parent,'Administrator username',c.admin.username||'admin'),p=field(parent,required?'Password':'New password',required?(c.admin.password||''):'','password',required?'At least 12 characters. You will use this to sign in to Administration.':'Leave blank to keep your current password.');u.autocomplete='username';p.autocomplete='new-password';
 return()=>{if(required){if(!u.value.trim())invalid(u,'Enter an administrator username.');if(p.value.length<12)invalid(p,'Use at least 12 characters for your password.')}return {username:u.value.trim(),password:p.value}};
}
function scanFields(parent,c){
 const start=field(parent,'Scan when the server starts',c.scan.startup,'checkbox','Recommended. Finds new packages after a restart.'),interval=field(parent,'Check for new packages every',c.scan.interval_minutes,'number','Minutes. Use 15 for automatic checks, or 0 to scan manually.');interval.min='0';interval.step='1';
 return()=>({startup:start.checked,interval_minutes:Number(interval.value)});
}
function igdbFields(parent,c){
 parent.append(el('p','IGDB adds descriptions, genres and screenshots. It is optional: your packages and downloads work without it.','muted'));
 const enabled=field(parent,'Fetch game details from IGDB',c.igdb.enabled,'checkbox');
 const inputs=el('div'),id=field(inputs,'IGDB Client ID',c.igdb.client_id),secret=field(inputs,'IGDB Client Secret','','password','Leave blank to keep the saved secret.');parent.append(inputs);
 const link=el('a','How to get IGDB credentials');link.href='https://api-docs.igdb.com/#account-creation';link.target='_blank';link.rel='noopener noreferrer';inputs.append(link);
 const toggle=()=>{inputs.hidden=!enabled.checked};enabled.addEventListener('change',toggle);toggle();
 return()=>{if(enabled.checked&&!id.value.trim())invalid(id,'Enter your IGDB Client ID, or turn off game details for now.');if(enabled.checked&&!secret.value&&!c.igdb.client_secret&&!savedIGDBSecret)invalid(secret,'Enter your IGDB Client Secret, or skip game details for now.');return {...c.igdb,enabled:enabled.checked,client_id:id.value.trim(),client_secret:secret.value||c.igdb.client_secret||''}};
}
async function setup(){
 heading.textContent='Server setup';let c;
 const token=new URLSearchParams(location.hash.slice(1)).get('token');
 if(token){const b=await api('setup/open','POST',{token});csrf=b.csrf;c=b.config;savedIGDBSecret=Boolean(b.igdb_secret_set);history.replaceState(null,'','/setup')}
 else{const b=await api('auth/session');if(!b.setup)throw Error('Open the full setup link from your server terminal or container logs, including #token=.');csrf=b.csrf;c=b.config;savedIGDBSecret=Boolean(b.igdb_secret_set)}
 if(!c.public_url&&!['localhost','127.0.0.1','[::1]'].includes(location.hostname))c.public_url=location.origin;
 let step=0;const shell=el('div',undefined,'wizard'),stepsList=el('ol',undefined,'wizard-steps'),panel=el('div',undefined,'panel wizard-panel');shell.append(stepsList,panel);content.append(shell);const save={};
 const steps=[
 ['Account','Create your administrator account','Protect settings with a username and password. Your PS4 downloads will not need a login.',()=>{save.admin=credentials(panel,c,true)}],
 ['Folders','Choose your package folders','Add a folder containing PS4 .pkg files. Subfolders are scanned automatically.',()=>{if(!c.mappings.length)panel.append(el('p',c.data_dir==='/data'?'In this container, start with /packages. This is the games folder mounted by Compose.':'Choose folders on the computer running this server.','muted'));save.mappings=mappingEditor(panel,c)}],
 ['Connection','Make the server reachable','Use the address your PS4 can open on your home network.',()=>{save.general=generalFields(panel,c)}],
 ['Scanning','Keep your library current','Scans read package details without copying large game files.',()=>{save.scan=scanFields(panel,c)}],
 ['Game details','Add optional game details','You can skip this and enable it later in Administration.',()=>{save.igdb=igdbFields(panel,c)}],
 ['Review','Ready to build your library','Save your settings, then sign in to start scanning. Packages appear before optional game details finish.',()=>{
 const review=el('dl',undefined,'review');for(const [label,value]of [['Public URL',c.public_url],['Package folders',c.mappings.filter(m=>m.enabled).map(m=>m.path).join(', ')||'None yet — add a folder later'],['Data folder',c.data_dir],['Game details',c.igdb.enabled?'Enabled (runs after packages appear)':'Skipped']])review.append(el('dt',label),el('dd',value));panel.append(review);
 }]
 ];
 function draw(){message.textContent='';stepsList.replaceChildren();steps.forEach((entry,index)=>{const li=el('li',entry[0],index===step?'active':index<step?'done':'');if(index===step)li.setAttribute('aria-current','step');stepsList.append(li)});panel.replaceChildren(el('p',`Step ${step+1} of ${steps.length}`,'step-counter'),el('h2',steps[step][1]),el('p',steps[step][2],'step-description'));steps[step][3]();const actions=el('div',undefined,'actions');if(step>0)actions.append(button('Back',()=>{collect(false);step--;draw()}));
 if(step<steps.length-1)actions.append(button('Continue',async()=>{collect(true);if(step===1)for(const m of c.mappings.filter(m=>m.enabled)){try{await api('directories?path='+encodeURIComponent(m.path))}catch(e){throw Error(`Cannot use ${m.path}: ${e.message}`)}}step++;draw()},'primary'));
 else actions.append(button('Save and start',async()=>{const result=await api('setup','PUT',c);location.href='/login?first=1'+(result.restart_required?'&restart=1':'')},'primary'));panel.append(actions);
 }
 function collect(validate){
 if(step===0){if(validate)c.admin=save.admin();else{const inputs=panel.querySelectorAll('input');c.admin={username:inputs[0].value,password:inputs[1].value}}}
 if(step===1){c.mappings=save.mappings();if(validate){const ids=new Set();for(const m of c.mappings){if(!/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$/.test(m.id)||!/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,79}$/.test(m.category)||ids.has(m.id))throw Error('Use distinct folder identifiers and simple feed names: letters, numbers, underscores or hyphens.');ids.add(m.id);if(m.enabled&&!m.path)throw Error('Select a package folder, or remove the empty folder row.')}}}
 if(step===2){Object.assign(c,save.general());if(validate){let u;try{u=new URL(c.public_url)}catch{throw Error('Enter a full public URL, for example http://192.168.1.20:8080.')}if(!['http:','https:'].includes(u.protocol)||u.username||u.password||u.search||u.hash||u.pathname!=='/'||['0.0.0.0','[::]','*'].includes(u.hostname))throw Error('Enter the HTTP or HTTPS address of your server, with its port and without a path.');if(!c.data_dir)throw Error('Enter a writable application data folder. In Docker, keep /data.')}}
 if(step===3){c.scan=save.scan();if(validate&&(!Number.isSafeInteger(c.scan.interval_minutes)||c.scan.interval_minutes<0))throw Error('Enter a whole number of minutes: 15 for automatic checks, or 0 for manual scanning.')}
 if(step===4){if(validate)c.igdb=save.igdb();else{const fields=panel.querySelectorAll('input');c.igdb={...c.igdb,enabled:fields[0].checked,client_id:fields[1].value,client_secret:fields[2].value||c.igdb.client_secret||''}}}
 }draw();
}
async function admin(){
 let refreshing=false;heading.textContent='Administration';const session=await api('auth/session');csrf=session.csrf;savedIGDBSecret=Boolean(session.igdb_secret_set);document.querySelector('#logout').hidden=false;const c=await api('settings');feedURLs=await api('feeds');
 const statusPanel=el('div',undefined,'panel'),scan=scanProgress(),stats=el('div',undefined,'stats'),actions=el('div',undefined,'row');scan.box.className='';
 actions.append(button('Scan now',async()=>{await api('scan','POST',{force:false});await refresh()},'primary'),button('Read all packages again',async()=>{await api('scan','POST',{force:true});await refresh()}),button('Stop scan',async()=>{await api('scan/cancel','POST',{});notify('Stopping the scan. Packages already in the library stay available.');await refresh()}));statusPanel.append(scan.box,stats,actions);content.append(statusPanel);
 const logPanel=el('div',undefined,'panel'),logs=el('pre',undefined,'server-log');logs.setAttribute('role','log');logs.setAttribute('aria-label','Server log');logs.setAttribute('aria-live','off');logs.tabIndex=0;logPanel.append(el('h2','Server log'),logs);content.append(logPanel);
 const settings=el('div',undefined,'panel'),columns=el('div',undefined,'admin-grid'),general=el('div'),folders=el('div');general.append(el('h2','Server settings'));const getGeneral=generalFields(general,c);const account=el('details');account.append(el('summary','Administrator account'));const getAdmin=credentials(account,c);general.append(account);
 const scanning=el('details');scanning.append(el('summary','Automatic scanning'));const getScan=scanFields(scanning,c);general.append(scanning);folders.append(el('h2','Package folders'));const getMappings=mappingEditor(folders,c);
 const igdb=el('details');igdb.append(el('summary','Optional game details'));const getIGDB=igdbFields(igdb,c);general.append(igdb);columns.append(general,folders);settings.append(columns,button('Save settings',async()=>{const next={...c,...getGeneral(),admin:getAdmin(),scan:getScan(),mappings:getMappings(),igdb:getIGDB()};const b=await api('settings','PUT',next);notify(b.restart_required?'Saved. Restart the server to apply network or data-folder changes.':'Saved. Folder and scan changes apply to the next scan.');if(b.login_required)location.href='/login'},'primary'));content.append(settings);
 const overrides=el('details',undefined,'panel');overrides.append(el('summary','Advanced: metadata overrides'),el('p','A JSON object keyed by Title ID. Example: {"CUSA12345":{"summary":"My description"}}. Run a scan after saving.','muted'));const text=area(overrides,'Overrides JSON',JSON.stringify(await api('metadata/overrides'),null,2));overrides.append(button('Save overrides',async()=>{let parsed;try{parsed=JSON.parse(text.value)}catch{throw Error('Overrides must be valid JSON. Check commas, quotes and braces.')}const b=await api('metadata/overrides','PUT',parsed);notify(b.status)}));content.append(overrides);
 content.append(feedList(await api('library')));
 async function refresh(){if(refreshing)return;refreshing=true;try{const b=await api('status','GET',undefined,true);scan.update(b.scan,b.packages);stats.replaceChildren();for(const [n,label]of [[b.packages,'available packages'],[b.scan.inspected,'read this scan'],[b.scan.reused,'reused from cache'],[b.scan.errors.length,'warnings']]){const stat=el('div',undefined,'stat');stat.append(el('strong',n||0),el('span',label));stats.append(stat)}const lines=await api('logs','GET',undefined,true),text=lines.join('\n')||'No server messages yet.';if(logs.textContent!==text){logs.textContent=text;logs.scrollTop=logs.scrollHeight}if(b.restart_required)notify('Settings saved. Restart the server to apply network or data-folder changes.')}catch(e){notify(e.message,true)}finally{refreshing=false}}
 await refresh();setInterval(refresh,2000);
}
document.querySelector('#logout').onclick=async()=>{try{await api('auth/logout','POST',{});location.href='/login'}catch(e){notify(e.message,true)}};
const pageLoading=el('div',undefined,'page-loading'),pageLoadingBar=el('progress');pageLoadingBar.setAttribute('aria-label','Loading page data');pageLoading.append(pageLoadingBar,el('p','Loading data from your server…'),el('small','This may take a moment on a NAS. The page will update when it is ready.'));content.append(pageLoading);
(async()=>{try{if(location.pathname==='/setup')await setup();else if(location.pathname==='/login')await login();else if(location.pathname==='/admin'){try{await admin()}catch(e){if(e.message.includes('login required')||e.message.includes('sign in'))location.href='/login';else throw e}}else await library()}catch(e){notify(e.message,true)}finally{pageLoading.remove()}})();
