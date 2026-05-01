package main
import ("fmt";"log";sc "serato-songcompanion/seratosync")
func main(){
  path:="Sessions/36.session";ch,err:=sc.ReadFileChunks(path); if err!=nil{log.Fatal(err)}
  idx:=0
  for _,c:=range ch{ if c.Tag!="oent"{continue}; idx++;
    adats:=sc.ExtractADATPayloads(c.Data)
    for _,a:= range adats{
      strs:=sc.ExtractUTF16Strings(a,2)
      if len(strs)==0 {continue}
      if len(strs)==1 && strs[0]=="Offline Player"{continue}
      fmt.Printf("oent#%d adat strings(%d):\n",idx,len(strs))
      for i,s:= range strs{ if i<8{ fmt.Println("  ", s) } }
      return
    }
  }
  fmt.Println("no interesting strings found")
}
