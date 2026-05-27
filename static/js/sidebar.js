(function(){
  var currentPath=document.body.dataset.currentPath||"/";
  var container=document.getElementById("tree-root");
  if(!container)return;

  fetch("/__mdview/tree.json").then(function(r){return r.json()}).then(function(tree){
    var frag=renderNode(tree,0);
    if(frag)container.appendChild(frag);
    var active=document.querySelector(".tree-file.active");
    if(active)active.scrollIntoView({block:"nearest"});
  });

  function renderNode(node,depth){
    if(node.isDir){
      var wrap=document.createElement("div");
      wrap.className="tree-item";
      var btn=document.createElement("button");
      btn.className="tree-folder collapsed";
      btn.style.paddingLeft=(12+depth*16)+"px";
      btn.textContent=node.name;
      var kids=document.createElement("div");
      kids.className="tree-children collapsed";
      var hasActive=false;
      if(node.children){
        node.children.sort(function(a,b){
          var ad=a.isDir||false,bd=b.isDir||false;
          if(ad!==bd)return ad?-1:1;
          return(a.name<b.name)?-1:(a.name>b.name)?1:0;
        });
        for(var i=0;i<node.children.length;i++){
          var child=renderNode(node.children[i],depth+1);
          if(child){
            kids.appendChild(child);
            if(child.dataset.hasActive)hasActive=true;
          }
        }
      }
      btn.onclick=function(){
        btn.classList.toggle("collapsed");
        kids.classList.toggle("collapsed");
      };
      wrap.appendChild(btn);
      wrap.appendChild(kids);
      if(hasActive){
        btn.classList.remove("collapsed");
        kids.classList.remove("collapsed");
        wrap.dataset.hasActive="1";
      }
      return wrap;
    }else{
      var wrap=document.createElement("div");
      wrap.className="tree-item";
      var link=document.createElement("a");
      link.className="tree-file";
      link.href=node.path;
      link.style.paddingLeft=(12+depth*16)+"px";
      link.textContent=node.name;
      if(node.path===currentPath){
        link.classList.add("active");
        wrap.dataset.hasActive="1";
      }
      wrap.appendChild(link);
      return wrap;
    }
  }

  var toggle=document.getElementById("sidebar-toggle");
  if(toggle){
    toggle.onclick=function(){
      var sb=document.querySelector(".mdview-sidebar");
      if(window.innerWidth<=767){
        sb.classList.toggle("sidebar-open");
        var ov=document.querySelector(".sidebar-overlay");
        if(ov)ov.classList.toggle("visible");
      }else{
        document.body.classList.toggle("sidebar-collapsed");
      }
    };
  }

  var ov=document.querySelector(".sidebar-overlay");
  if(ov){
    ov.onclick=function(){
      var sb=document.querySelector(".mdview-sidebar");
      sb.classList.remove("sidebar-open");
      ov.classList.remove("visible");
    };
  }
})();
