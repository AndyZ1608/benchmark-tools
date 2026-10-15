#define _POSIX_C_SOURCE 200809L
#include <stdio.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <time.h>
#include <pthread.h>
#include <stdatomic.h>
#include <math.h>
#define VERSION "compute-bench-v1"
static double now(void){struct timespec t;clock_gettime(CLOCK_MONOTONIC,&t);return t.tv_sec+t.tv_nsec/1e9;}
static _Atomic uint64_t sink;
static volatile uint64_t seed_input=1;
static volatile double fp_input=.5;
static uint64_t integer(uint64_t x){for(int i=0;i<4096;i++){x^=x<<13;x^=x>>7;x^=x<<17;}return x;}
static double floating(void){double x=fp_input;for(int i=0;i<4096;i++)x=x*0.999999+0.000001;return x;}
typedef struct {int id,workers,kind;size_t n;double seconds,warmup,elapsed,value;uint64_t count,errors;double *a,*b,*c;size_t *chain;pthread_barrier_t *barrier;} Job;
static void cpu_batch(Job *j){if(j->kind==0){uint64_t x=integer(seed_input);sink=x;j->errors+=x!=UINT64_C(15846333869755411255);}else{double x=floating();sink=(uint64_t)(x*1e12);j->errors+=!isfinite(x)||fabs(x-(1-.5*pow(.999999,4096)))>1e-10;}j->count+=4096;}
/* Volatile dependent loads prevent an optimizer from deleting pointer chasing
   when the otherwise-unread atomic sink is discarded. A full known cycle ends at 0. */
static void mem_batch(Job *j){size_t n=j->n;switch(j->kind){case 2:for(size_t i=0;i<n;i++)j->c[i]=j->a[i];break;case 3:for(size_t i=0;i<n;i++)j->b[i]=3*j->c[i];break;case 4:for(size_t i=0;i<n;i++)j->c[i]=j->a[i]+j->b[i];break;case 5:for(size_t i=0;i<n;i++)j->a[i]=j->b[i]+3*j->c[i];break;case 6:{size_t x=0;const volatile size_t *links=j->chain;for(size_t i=0;i<n;i++)x=links[x];sink=x;j->errors+=x!=0;break;}case 7:for(size_t i=0;i<n;i++)j->a[i]=(double)i;for(size_t i=0;i<n;i++)j->errors+=((volatile double*)j->a)[i]!=(double)i;break;}j->count++;}
static void *worker(void *v){Job *j=v;double end=now()+j->warmup;do{if(j->kind<2)cpu_batch(j);else mem_batch(j);}while(now()<end);pthread_barrier_wait(j->barrier);j->count=0;double start=now();end=start+j->seconds;do{if(j->kind<2)cpu_batch(j);else mem_batch(j);}while(now()<end);j->elapsed=now()-start;if(j->kind>=2&&j->kind<=5){double expected=j->kind==2?1:j->kind==3?3:j->kind==4?3:5;double *a=j->kind==3?j->b:j->kind==5?j->a:j->c;for(size_t i=0;i<j->n;i++)j->errors+=a[i]!=expected;}return NULL;}
int main(int argc,char **argv){if(argc==2&&!strcmp(argv[1],"--version")){puts(VERSION);return 0;}if(argc!=6){fprintf(stderr,"usage: compute-bench cpu|ram|integrity workers seconds warmup bytes\n");return 2;}int w=atoi(argv[2]);double seconds=atof(argv[3]),warm=atof(argv[4]);char *end;unsigned long long bytes=strtoull(argv[5],&end,10);if(!strcmp(end,"MiB"))bytes*=1ULL<<20;else if(!strcmp(end,"GiB"))bytes*=1ULL<<30;else if(!strcmp(end,"KiB"))bytes*=1ULL<<10;else if(*end)return 2;if(w<1||w>4096||seconds<1||seconds>86400||warm<0||warm>3600||bytes<4096||bytes>1ULL<<40)return 2;int first,last;if(!strcmp(argv[1],"cpu")){first=0;last=1;}else if(!strcmp(argv[1],"ram")){first=2;last=6;}else if(!strcmp(argv[1],"integrity")){first=last=7;}else return 2;
/* Independent fixed known-answer check: xorshift64 from seed 1. */
uint64_t ref=1;for(int i=0;i<4096;i++){ref^=ref<<13;ref^=ref>>7;ref^=ref<<17;}if(integer(1)!=ref)return 3;
const char *names[]={"integer-xorshift64","fp-multiply-add","copy","scale","add","triad","dependent-pointer-chase","integrity"};printf("{\"kernel\":\"%s\",\"workers\":%d,\"dataset_bytes\":%llu,\"measurements\":[",VERSION,w,bytes);
for(int k=first;k<=last;k++){Job *jobs=calloc(w,sizeof(Job));pthread_t *threads=calloc(w,sizeof(pthread_t));pthread_barrier_t barrier;if(!jobs||!threads||pthread_barrier_init(&barrier,NULL,w))return 4;size_t n=(size_t)(bytes/w/24);if(n<128)return 2;uint64_t errors=0,count=0;double elapsed=0,latency_sum=0;
for(int i=0;i<w;i++){Job *j=&jobs[i];*j=(Job){.id=i,.workers=w,.kind=k,.n=n,.seconds=seconds,.warmup=warm,.barrier=&barrier};if(k>=2){if(posix_memalign((void**)&j->a,64,n*8)||posix_memalign((void**)&j->b,64,n*8)||posix_memalign((void**)&j->c,64,n*8))return 4;for(size_t x=0;x<n;x++){j->a[x]=1;j->b[x]=2;j->c[x]=1;}if(k==6){j->chain=(size_t*)j->a;size_t *order=malloc(n*sizeof(size_t));if(!order)return 4;for(size_t x=0;x<n;x++)order[x]=x;uint64_t seed=42;for(size_t x=n-1;x>0;x--){seed=seed*6364136223846793005ULL+1;size_t y=seed%(x+1),t=order[x];order[x]=order[y];order[y]=t;}for(size_t x=0;x<n;x++)j->chain[order[x]]=order[(x+1)%n];free(order);}}
if(pthread_create(&threads[i],NULL,worker,j))return 4;}
for(int i=0;i<w;i++){pthread_join(threads[i],NULL);errors+=jobs[i].errors;count+=jobs[i].count;if(k==6)latency_sum+=jobs[i].elapsed*1e9/((double)jobs[i].count*n);if(jobs[i].elapsed>elapsed)elapsed=jobs[i].elapsed;free(jobs[i].a);free(jobs[i].b);free(jobs[i].c);}double value=(double)count/elapsed;const char *unit="defined_iterations/s";if(k>=2&&k<=5){value*=n*(k<=3?16:24)/1e9;unit="GB/s";}if(k==6){value=latency_sum/w;unit="ns/access mean per worker";}if(k==7){value*=n;unit="verified_elements/s";}
printf("%s{\"workload\":\"%s\",\"value\":%.9g,\"unit\":\"%s\",\"elapsed_seconds\":%.9g,\"integrity_errors\":%llu}",k==first?"":",",names[k],value,unit,elapsed,(unsigned long long)errors);pthread_barrier_destroy(&barrier);free(jobs);free(threads);if(errors)return 3;}
puts("]}");return 0;}
