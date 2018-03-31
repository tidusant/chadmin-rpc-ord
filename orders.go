package main

import (
	"time"

	"github.com/tidusant/c3m-common/c3mcommon"
	"github.com/tidusant/c3m-common/inflect"
	"github.com/tidusant/c3m-common/log"
	"github.com/tidusant/c3m-common/lzjs"
	"github.com/tidusant/c3m-common/mycrypto"
	"github.com/tidusant/c3m-common/mystring"
	rpch "github.com/tidusant/chadmin-repo/cuahang"
	"github.com/tidusant/chadmin-repo/models"
	"gopkg.in/mgo.v2/bson"

	"encoding/base64"
	"encoding/json"
	"math"

	"flag"
	"net"
	"net/rpc"
	"strconv"
	"strings"
)

const (
	defaultcampaigncode string = "XVsdAZGVmY "
)

type Arith int

func (t *Arith) Run(data string, result *string) error {
	log.Debugf("Call RPC orders args:" + data)
	*result = ""
	//parse args
	args := strings.Split(data, "|")

	if len(args) < 3 {
		return nil
	}
	var usex models.UserSession
	usex.Session = args[0]
	usex.Action = args[2]
	info := strings.Split(args[1], "[+]")
	usex.UserID = info[0]
	ShopID := info[1]
	usex.Params = ""
	if len(args) > 3 {
		usex.Params = args[3]
	}

	//	if usex.Action == "c" {
	//		*result = CreateProduct(usex)

	//	} else

	//check shop permission
	shop := rpch.GetShopById(usex.UserID, ShopID)
	if shop.Status == 0 {
		*result = c3mcommon.ReturnJsonMessage("-4", "Shop is disabled.", "", "")
		return nil
	}
	usex.Shop = shop
	if usex.Action == "statusc" {
		*result = LoadAllStatusCount(usex)
	} else if usex.Action == "status" {
		*result = LoadAllStatus(usex)
	} else if usex.Action == "lao" {
		*result = LoadAllOrderByStatus(usex)
	} else if usex.Action == "lg" {
		*result = LoadCities(usex)
	} else if usex.Action == "us" {
		*result = UpdateOrderStatus(usex)
	} else if usex.Action == "ds" {
		*result = DeleteOrderStatus(usex)
	} else if usex.Action == "ss" {
		*result = SaveStatus(usex)
	} else if usex.Action == "uo" {
		*result = UpdateOrder(usex)
	} else { //default
		*result = c3mcommon.ReturnJsonMessage("-5", "Action not found.", "", "")
	}

	return nil
}

func parseOrder(order models.Order, usex models.UserSession, defaultstatus models.OrderStatus, shipper models.Shipper) {
	if order.C == "" {
		return
	}
	//loop to get item
	orderc := order.C

	for {
		if len(orderc) < 3 {
			break
		}

		code := orderc[:3]
		orderc = orderc[3:]
		//get prod
		prod := rpch.GetProdByCode(usex.Shop.ID.Hex(), code)
		if prod.Code == "" {
			//prod not found
			break
		}

		//get num
		num := 1
		//loop to get num
		numstr := ""
		for {
			if len(orderc) <= 0 {
				break
			}
			str := orderc[:1]
			if !mystring.IsInt(str) {
				break
			}
			numstr = numstr + str
			orderc = orderc[1:]
		}

		//check num
		if numstr != "" {
			num, _ = strconv.Atoi(numstr)
		}

		//create order item
		var item models.OrderItem
		item.Code = prod.Code
		item.BasePrice = prod.Langs[order.L].BasePrice
		item.Price = prod.Langs[order.L].Price
		item.Title = prod.Langs[order.L].Name
		item.Avatar = prod.Langs[order.L].Avatar
		item.Num = num
		order.Items = append(order.Items, item)
		order.Total += item.Price
		order.BaseTotal += item.BasePrice
		order.ShipFee = usex.Shop.Config.ShipFee
		order.PartnerShipFee = order.ShipFee
		if usex.Shop.Config.FreeShip <= order.Total {
			order.ShipFee = 0
		}
	}
	order.Status = defaultstatus.ID.Hex()
	rpch.SaveOrder(order)
}
func LoadAllOrderByStatus(usex models.UserSession) string {

	args := strings.Split(usex.Params, ",")
	status := args[0]
	page := 1
	pagesize := 10
	count := 1
	searchterm := ""
	if len(args) > 2 {
		searchterm = args[2]
		if searchterm != "" {
			byteDecode, _ := base64.StdEncoding.DecodeString(mycrypto.Base64fix(searchterm))
			searchterm = string(byteDecode)
			log.Debugf("searchterm: %s", searchterm)
		}
	}

	page, _ = strconv.Atoi(args[1])

	if page == 1 {
		count = rpch.CountOrdersByStatus(usex.Shop.ID.Hex(), status, searchterm)
	}
	//update order from web
	orders := rpch.GetOrdersByStatus(usex.Shop.ID.Hex(), "", 0, pagesize, "")
	//default status
	defaultstatus := rpch.GetDefaultOrderStatus(usex.Shop.ID.Hex())
	//default shipper
	defaultshipper := rpch.GetDefaultShipper(usex.Shop.ID.Hex())
	//all campaign
	camps := rpch.GetAllCampaigns(usex.Shop.ID.Hex())
	mapcamp := make(map[string]string)
	for _, v := range camps {
		mapcamp[v.ID.Hex()] = v.Name
	}
	for _, order := range orders {
		parseOrder(order, usex, defaultstatus, defaultshipper)
	}
	orders = rpch.GetOrdersByStatus(usex.Shop.ID.Hex(), status, page, pagesize, searchterm)
	cuss := make(map[string]models.Customer)
	for k, v := range orders {
		//get cus
		if _, ok := cuss[v.Phone]; !ok {
			cuss[v.Phone] = rpch.GetCusByPhone(v.Phone, usex.Shop.ID.Hex())
		}
		orders[k].Name = cuss[v.Phone].Name
		if campname, ok := mapcamp[orders[k].CampaignId]; ok {
			orders[k].CampaignName = campname
		}

		orders[k].Email = cuss[v.Phone].Email
		orders[k].City = cuss[v.Phone].City
		orders[k].District = cuss[v.Phone].District
		orders[k].Ward = cuss[v.Phone].Ward
		orders[k].Address = cuss[v.Phone].Address
		orders[k].CusNote = cuss[v.Phone].Note
		orders[k].OrderCount = rpch.CountOrderByCus(v.Phone, usex.Shop.ID.Hex())
		orders[k].SearchIndex = ""

	}
	info, _ := json.Marshal(orders)
	strrt := `{"rs":` + string(info) + `,"pagecount":` + strconv.Itoa((int)(math.Ceil(float64(count)/float64(pagesize)))) + `}`
	//strrt = string(info)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}
func LoadAllStatus(usex models.UserSession) string {

	//default status
	status := rpch.GetAllOrderStatus(usex.Shop.ID.Hex())

	info, _ := json.Marshal(status)

	strrt := string(info)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}
func LoadAllStatusCount(usex models.UserSession) string {

	//default status
	status := rpch.GetAllOrderStatus(usex.Shop.ID.Hex())
	// //all campaign
	// camps := rpch.GetAllCampaigns(usex.Shop.ID.Hex())
	// mapcamp := make(map[string]string)
	// for _, v := range camps {
	// 	mapcamp[v.ID.Hex()] = v.Name
	// }
	// //all shipper
	// shippers := rpch.GetAllShipper(usex.Shop.ID.Hex())
	// mapshipper := make(map[string]string)
	// for _, v := range shippers {
	// 	mapshipper[v.ID.Hex()] = v.Name
	// }
	//update order from web
	for k, v := range status {
		status[k].OrderCount = rpch.CountOrdersByStatus(usex.Shop.ID.Hex(), v.ID.Hex(), "")
		// ords := rpch.GetOrdersByStatus(usex.Shop.ID.Hex(), v.ID.Hex(), 1, 100000, "")
		// cuss := make(map[string]models.Customer)
		// for _, ord := range ords {
		// 	//get cus
		// 	if _, ok := cuss[ord.Phone]; !ok {
		// 		cuss[ord.Phone] = rpch.GetCusByPhone(ord.Phone, usex.Shop.ID.Hex())
		// 	}
		// 	ord.CampaignName = mapcamp[ord.CampaignId]
		// 	ord.SearchIndex = inflect.ParameterizeJoin(ord.Name+ord.Phone+ord.City+ord.District+ord.Ward+ord.Address+ord.CusNote+ord.Note+ord.ShipmentCode+mapcamp[ord.CampaignId]+cuss[ord.Phone].Name+cuss[ord.Phone].City+cuss[ord.Phone].District+cuss[ord.Phone].Ward+cuss[ord.Phone].Address+cuss[ord.Phone].Email+cuss[ord.Phone].Note+mapshipper[ord.ShipperId], " ")
		// 	rpch.SaveOrder(ord)
		// }

	}
	info, _ := json.Marshal(status)

	strrt := string(info)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}
func LoadCities(usex models.UserSession) string {

	//default status

	//update order from web
	cities := rpch.GetCities()
	//update order from web

	citiesb, _ := json.Marshal(cities)
	strrt := string(citiesb)
	return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
}

func UpdateOrderStatus(usex models.UserSession) string {

	info := strings.Split(usex.Params, ",")
	cancelPartner := "0"
	if len(info) > 1 {
		changestatusid := info[1]
		orderid := info[0]

		//check cancel ghtk status:
		status := rpch.GetStatusByID(changestatusid, usex.Shop.ID.Hex())
		ghtkstatussync := status.PartnerStatus["ghtk"]
		if ghtkstatussync != nil {
			for _, statcode := range ghtkstatussync {
				if statcode == "-1" {
					cancelPartner = "1"
				}
			}
		}
		//check stock
		order := rpch.GetOrderByID(orderid, usex.Shop.ID.Hex())
		if order.Status == "" {
			return c3mcommon.ReturnJsonMessage("-5", "order not found", "", "")
		}
		oldstat := rpch.GetStatusByID(order.Status, usex.Shop.ID.Hex())
		if oldstat.Export != status.Export {
			//update stock
			sign := 1
			if oldstat.Export {
				sign = -1
			}
			for _, v := range order.Items {
				if !rpch.ExportItem(usex.Shop.ID.Hex(), v.ProdCode, v.Code, v.Num*sign) {
					titleb, _ := lzjs.DecompressFromBase64(v.Title)
					return c3mcommon.ReturnJsonMessage("-5", "Out of stock: "+string(titleb), "", "")
				}
			}
		}
		rpch.UpdateOrderStatus(usex.Shop.ID.Hex(), changestatusid, orderid)

	}

	return c3mcommon.ReturnJsonMessage("1", "", cancelPartner, "")
}

func SaveStatus(usex models.UserSession) string {

	var status models.OrderStatus
	err := json.Unmarshal([]byte(usex.Params), &status)
	if !c3mcommon.CheckError("update status parse json", err) {
		return c3mcommon.ReturnJsonMessage("0", "update status fail", "", "")
	}
	//check old status
	oldstat := status
	if oldstat.ID.Hex() != "" {
		oldstat = rpch.GetStatusByID(status.ID.Hex(), usex.Shop.ID.Hex())
		oldstat.Title = status.Title
		oldstat.Color = status.Color
		oldstat.Default = status.Default
		oldstat.Finish = status.Finish
		oldstat.Export = status.Export
		oldstat.PartnerStatus = status.PartnerStatus
	} else {
		oldstat.UserId = usex.UserID
		oldstat.ShopId = usex.Shop.ID.Hex()
	}

	//check default
	if oldstat.Default == true {
		rpch.UnSetStatusDefault(usex.Shop.ID.Hex())
	}
	if oldstat.Color == "" {
		oldstat.Color = "ffffff"
	}

	oldstat = rpch.SaveOrderStatus(oldstat)
	b, _ := json.Marshal(oldstat)
	return c3mcommon.ReturnJsonMessage("1", "", "success", string(b))
}

func DeleteOrderStatus(usex models.UserSession) string {
	//get stat
	stat := rpch.GetStatusByID(usex.Params, usex.Shop.ID.Hex())
	if stat.ID.Hex() == "" {
		return c3mcommon.ReturnJsonMessage("-5", "Status not found.", "", "")
	}
	if stat.Default {
		return c3mcommon.ReturnJsonMessage("-5", "Status is default. Please select another status default.", "", "")
	}
	//check status empty
	count := rpch.GetCountOrderByStatus(stat)
	//check old status
	if count > 0 {
		return c3mcommon.ReturnJsonMessage("-5", "Status not empty. "+strconv.Itoa(count)+" orders use this status", "", "")
	}

	rpch.DeleteOrderStatus(stat)

	return c3mcommon.ReturnJsonMessage("1", "", "success", "")
}

func UpdateOrder(usex models.UserSession) string {
	shop := rpch.GetShopById(usex.UserID, usex.Shop.ID.Hex())
	if shop.Status == 0 {
		return c3mcommon.ReturnJsonMessage("-4", "Shop is disabled.", "", "")
	}
	var order models.Order
	err := json.Unmarshal([]byte(usex.Params), &order)
	if !c3mcommon.CheckError("update order parse json", err) {
		return c3mcommon.ReturnJsonMessage("0", "update order fail", "", "")
	}
	oldorder := order
	mapolditems := make(map[string]models.OrderItem)
	if order.ID.Hex() != "" {
		oldorder = rpch.GetOrderByID(order.ID.Hex(), shop.ID.Hex())
		for _, v := range oldorder.Items {
			mapolditems[v.Code] = v
		}
	} else {
		oldorder.ShopId = usex.Shop.ID.Hex()
		oldorder.ID = bson.NewObjectId()
		oldorder.Created = time.Now().Unix()
		oldorder.Modified = oldorder.Created
	}

	//all campaign
	camps := rpch.GetAllCampaigns(usex.Shop.ID.Hex())
	mapcamp := make(map[string]string)
	for _, v := range camps {
		mapcamp[v.ID.Hex()] = v.Name
	}
	//all shipper
	shippers := rpch.GetAllShipper(usex.Shop.ID.Hex())
	mapshipper := make(map[string]string)
	for _, v := range shippers {
		mapshipper[v.ID.Hex()] = v.Name
	}
	stats := rpch.GetAllOrderStatus(usex.Shop.ID.Hex())
	mapstat := make(map[string]models.OrderStatus)
	for _, v := range stats {
		mapstat[v.ID.Hex()] = v
	}

	//check export status
	if mapstat[order.Status].Export {
		//decrease stock
		for _, v := range order.Items {
			olditemcount := 0
			if _, ok := mapolditems[v.Code]; ok {
				olditemcount = mapolditems[v.Code].Num
			}
			if !(rpch.ExportItem(usex.Shop.ID.Hex(), v.ProdCode, v.Code, v.Num-olditemcount)) {
				titleb, _ := lzjs.DecompressFromBase64(v.Title)
				return c3mcommon.ReturnJsonMessage("-5", "Out of stock: "+string(titleb), "", "")
			}
		}
	}
	var cus models.Customer
	if oldorder.Phone == order.Phone {
		cus = rpch.GetCusByPhone(order.Phone, shop.ID.Hex())
	} else if oldorder.Phone == "" && oldorder.Email == order.Email {
		cus = rpch.GetCusByEmail(order.Email, shop.ID.Hex())
	}
	cus.Phone = order.Phone
	cus.Name = order.Name
	cus.City = order.City
	cus.District = order.District
	cus.Ward = order.Ward
	cus.Address = order.Address
	cus.Email = order.Email
	cus.Note = order.CusNote
	cus.ShopId = shop.ID.Hex()
	if rpch.SaveCus(cus) {
		//save order
		oldorder.City = order.City
		oldorder.District = order.District
		oldorder.Ward = order.Ward
		oldorder.Address = order.Address
		oldorder.Name = order.Name
		oldorder.Email = order.Email
		oldorder.Phone = order.Phone
		oldorder.CusNote = order.CusNote

		oldorder.Items = order.Items
		oldorder.BaseTotal = order.BaseTotal
		if order.CampaignId != "" {
			oldorder.CampaignId = order.CampaignId
			oldorder.CampaignName = order.CampaignName
		}
		if oldorder.Whookupdate == 0 {
			oldorder.Whookupdate = oldorder.Modified
		}
		oldorder.ShipperId = order.ShipperId
		oldorder.Note = order.Note
		oldorder.PartnerShipFee = order.PartnerShipFee
		oldorder.ShipFee = order.ShipFee
		oldorder.ShipmentCode = order.ShipmentCode
		oldorder.Total = order.Total
		oldorder.IsPaid = order.IsPaid
		oldorder.SearchIndex = inflect.ParameterizeJoin(oldorder.Name+oldorder.Email+oldorder.Phone+oldorder.City+oldorder.District+oldorder.Ward+oldorder.Address+oldorder.CusNote+oldorder.Note+oldorder.ShipmentCode+oldorder.CampaignName+mapshipper[oldorder.ShipperId], " ")
		if mapstat[oldorder.Status].Finish {
			oldorder.Whookupdate = time.Now().Unix()
		}

		rpch.SaveOrder(oldorder)
		oldorder.SearchIndex = ""
		info, _ := json.Marshal(oldorder)
		strrt := string(info)

		return c3mcommon.ReturnJsonMessage("1", "", "success", strrt)
	}

	return c3mcommon.ReturnJsonMessage("-5", "", "", "")
}
func main() {
	var port int
	var debug bool
	flag.IntVar(&port, "port", 9884, "help message for flagname")
	flag.BoolVar(&debug, "debug", false, "Indicates if debug messages should be printed in log files")
	flag.Parse()

	// logLevel := log.DebugLevel
	// if !debug {
	// 	logLevel = log.InfoLevel

	// }

	// log.SetOutputFile(fmt.Sprintf("adminOrder-"+strconv.Itoa(port)), logLevel)
	// defer log.CloseOutputFile()
	// log.RedirectStdOut()

	//init db
	arith := new(Arith)
	rpc.Register(arith)
	log.Infof("running with port:" + strconv.Itoa(port))

	tcpAddr, err := net.ResolveTCPAddr("tcp", ":"+strconv.Itoa(port))
	c3mcommon.CheckError("rpc dail:", err)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	c3mcommon.CheckError("rpc init listen", err)

	for {
		conn, err := listener.Accept()
		if err != nil {
			continue
		}
		go rpc.ServeConn(conn)
	}
}
