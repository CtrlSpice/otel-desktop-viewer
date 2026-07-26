var e=e=>e;function t(e){return e<.5?4*e*e*e:.5*(2*e-2)**3+1}function n(t,{delay:n=0,duration:r=400,easing:i=e}={}){let a=+getComputedStyle(t).opacity;return{delay:n,duration:r,easing:i,css:e=>`opacity: ${e*a}`}}function r(e,{delay:n=0,speed:r,duration:i,easing:a=t}={}){let o=e.getTotalLength(),s=getComputedStyle(e);return s.strokeLinecap!==`butt`&&(o+=parseInt(s.strokeWidth)),i===void 0?i=r===void 0?800:o/r:typeof i==`function`&&(i=i(o)),{delay:n,duration:i,easing:a,css:(e,t)=>`
			stroke-dasharray: ${o};
			stroke-dashoffset: ${t*o};
		`}}export{n,r as t};