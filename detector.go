package main

import (
	"bufio"
	"flag"
	"fmt"
	"github.com/miekg/dns"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	domainfile = flag.String("file", "", "domains file, read from stdin if not specify")
	dnserver   = flag.String("dns", "114.114.114.114", "recursive dns server")
	port       = flag.String("port", "53", "recursive dns server")
	verbose    = flag.Int("verbose", 1, "print verbose dns query info")
	batchNum   = flag.Int("batch", 100, "concurrent number")
	timeout    = flag.Int("timeout", 10, "timeout ")
	suffix     = flag.String("suffix", "cn.", "which cdn does this domain used")
)

func init() {
	flag.Parse()
}
func main() {
	qname := make([]string, 0, 100) //{"www.baidu.com", "www.chaoshanw.cn"}
	if *domainfile == "" {
		inputReader := bufio.NewReader(os.Stdin)
		for {
			input, err := inputReader.ReadString('\n')
			if err != nil {
				break
			}
			qname = append(qname, strings.Trim(input, "\n"))
		}
	} else {
		fp, err := os.Open(*domainfile)
		if err != nil {
			fmt.Printf("open %s failed", *domainfile)
			return
		}
		br := bufio.NewReader(fp)
		for {
			line, _, err := br.ReadLine()
			if err != nil {
				break
			}
			qname = append(qname, string(line))
		}
		fp.Close()
	}
	fmt.Printf("qname count %d\n", len(qname))

	nameserver := *dnserver + ":" + *port
	batch_query(qname, nameserver)
}

func query_one(nameserver, v string, control chan bool, wg *sync.WaitGroup, cdnD, retryD *[]string) {
	c := new(dns.Client)
	c.Net = "udp"
	c.Timeout = time.Duration(*timeout) * time.Second
	c.DialTimeout = 5 * time.Second
	c.ReadTimeout = 5 * time.Second
	c.WriteTimeout = 5 * time.Second
	m := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			Authoritative:     true,
			AuthenticatedData: true,
			CheckingDisabled:  false,
			RecursionDesired:  true,
			Opcode:            dns.OpcodeQuery,
		},
		Question: make([]dns.Question, 1),
	}
	qt := dns.TypeCNAME
	qc := uint16(dns.ClassINET)
	m.Question[0] = dns.Question{Name: dns.Fqdn(v), Qtype: qt, Qclass: qc}
	m.Id = dns.Id()
	r, _, err := c.Exchange(m, nameserver)
	if *verbose >= 1 {
		fmt.Println("start query ", v)
	}
	if err != nil {
		if *verbose >= 1 {
			fmt.Printf("%s error answer %s\n", v, err)
		}
		*retryD = append(*retryD, v)
	} else {
		if *verbose >= 3 {
			if r.Ns != nil && len(r.Ns) > 0 {
				fmt.Println(r.Ns[0].String())
			} else {
				fmt.Printf("domain %s has not ns record\n", v)
			}
		}
		if len(r.Answer) == 1 {
			dnsrr := r.Answer[0].String()
			if *verbose >= 1 {
				fmt.Println(dnsrr)
			}
			vv := strings.Split(dnsrr, "\t")
			result := vv[4]
			if strings.HasSuffix(result, *suffix) {
				if *verbose >= 1 {
					fmt.Printf("%s cname domain suffix is %s\n", v, *suffix)
				}
				*cdnD = append(*cdnD, v)
			}
		}
	}
	wg.Done()
	<-control
}
func batch_query(qname []string, nameserver string) {
	var wg sync.WaitGroup
	cdnDomains := make([]string, 0, 1000)
	retryDomains := make([]string, 0, 1000)
	control := make(chan bool, *batchNum)
	for {
		for _, v := range qname {
			wg.Add(1)
			go query_one(nameserver, v, control, &wg, &cdnDomains, &retryDomains)
			control <- true
		}
		wg.Wait()
		if *verbose >= 2 {
			fmt.Printf("!!!!!!%d domains need retry!!!!!!\n", len(retryDomains))
		}
		if *verbose >= 3 {
			for _, d := range retryDomains {
				fmt.Printf("%s\n", d)
			}
		}
		if len(retryDomains) == 0 {
			break
		}
		qname = retryDomains
		retryDomains = retryDomains[0:0]
	}
	if *verbose >= 1 {
		fmt.Printf("*****domains in cdn %s as follows******\n", *suffix)
		for _, d := range cdnDomains {
			fmt.Println(d)
		}
	}
	fmt.Printf("******total %d domains in cdn******\n", len(cdnDomains))

}