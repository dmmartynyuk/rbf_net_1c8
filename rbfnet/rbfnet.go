package rbfnet

import (
	"encoding/json"
	"log"
	"math"
	"sort"

	"gonum.org/v1/gonum/mat"
)

// GaussianKernel takes in a parameter for sigma (σ)
// and returns a valid (Gaussian) Radial Basis Function
// Kernel. If the input dimensions aren't valid, the
// kernel will return 0.0 (as if the vectors are orthogonal)
//
//     K(x, x`) = exp( -1 * |x - x`|^2 / 2σ^2)
//
// https://en.wikipedia.org/wiki/Radial_basis_function_kernel
//
// This can be used within any models that can use Kernels.
//
// Sigma (σ) will default to 1 if given 0.0
var mX []float64

//RBFNetwork сеть RBF для прогнозирования продаж. значения X (InputLayer) это точки времени 1,2,3 алиасы дней продажи
type RBFNetwork struct {
	Inputs       int       //количество входных нейронов = строки в матрице входных весов
	Hiddens      int       //количество скрытых нейронов = колонки в матрице входных весов
	Spreads      []float64 //Spreads Гауссовой функции от фходных значений. Размер Hiddens
	Centers      []float64 //центры гауссовых фунций, размерность равна количеству нейронов сети Hiddens
	Spread       float64   //ширина гауссовой функции, для всех ячеек одна
	WeightOutput []float64 //вычисляемые веса нейросети
	//LastChangeOutput []float64 //прогнозируемая величина, она же потом
}

//NewRBFNetwork создает сеть
func NewRBFNetwork(iInputs, iHiddens int, Spread float64, centers []float64) *RBFNetwork {
	rbf := &RBFNetwork{}
	rbf.Inputs = iInputs
	rbf.Hiddens = iHiddens
	rbf.Centers = make([]float64, iHiddens)
	copy(rbf.Centers, centers)
	rbf.Spreads = make([]float64, iHiddens)
	rbf.WeightOutput = make([]float64, iHiddens)
	rbf.Spread = Spread
	for i := 0; i < iHiddens; i++ {
		rbf.Spreads[i] = Spread
	}
	mX = make([]float64, iInputs*iHiddens)
	return rbf
}

//SetCenters устанавливает центры нейронов сети
func (rbf *RBFNetwork) SetCenters(centers []float64) {
	copy(rbf.Centers, centers)
}

//SetSpread устанавливает ширину функции Гаусса для каждого нейрона
func (rbf *RBFNetwork) SetSpread(Spread float64) {
	rbf.Spread = Spread
	for i := 0; i < rbf.Hiddens; i++ {
		rbf.Spreads[i] = Spread
	}
}

//SetISpread устанавливает ширину функции Гаусса для нейрона num
func (rbf *RBFNetwork) SetISpread(num int, spread float64) {
	rbf.Spreads[num] = spread
}

//GetISpread устанавливает ширину функции Гаусса для нейрона num
func (rbf *RBFNetwork) GetISpread(num int) float64 {
	return rbf.Spreads[num]
}

//GetSpreads получает значения Spread т.е ширину функции Гаусса для каждого нейрона сети
func (rbf *RBFNetwork) GetSpreads() []float64 {
	return rbf.Spreads
}

//GetW получает веса нейронов сети
func (rbf *RBFNetwork) GetW() []float64 {
	return rbf.WeightOutput
}

func matPrint(X mat.Matrix) {
	fa := mat.Formatted(X, mat.Prefix(""), mat.Squeeze())
	log.Printf("%v\n", fa)
}

/*
//DumpRBF запись настроек сети в файл
func (rbf *RBFNetwork) DumpRBF(fileName string) {
	outfile, err := os.OpenFile(fileName, os.O_CREATE|os.O_RDWR, 0777)
	if err != nil {
		panic("failed to dump the network to " + fileName)
	}
	defer outfile.Close()
	encoder := json.NewEncoder(outfile)
	encoder.Encode(rbf)
}
*/

//DumpRBF запись настроек сети в string
func (rbf *RBFNetwork) DumpRBF() []byte {
	jstring, _ := json.Marshal(rbf)
	return jstring
}

//LoadRBF загружает настройки сети из файла
func LoadRBF(jstring []byte) (*RBFNetwork, error) {
	/*
		infile, err := os.Open(fileName)
		if err != nil {
			panic("failed to load " + fileName)
		}
		defer infile.Close()
		decoder := json.NewDecoder(infile)
		rbf := &RBFNetwork{}
		decoder.Decode(rbf)
		return rbf
	*/
	rbf := &RBFNetwork{}
	if err := json.Unmarshal(jstring, rbf); err != nil {
		return rbf, err
	}
	return rbf, nil
}

//Gaussian возвращает функцию Гаусса
func Gaussian(x float64, center float64, sigma float64) float64 {
	if sigma == 0 {
		sigma = 1.0
	}
	denom := 2 * sigma * sigma
	var diff float64
	diff = (x - center) * (x - center)
	return math.Exp(-1 * diff / denom)

}

/*
func sigmoid(X float64) float64 {
	return 1.0 / (1.0 + math.Pow(math.E, -float64(X)))
}

func dsigmoid(Y float64) float64 {
	return Y * (1.0 - Y)
}
*/

//StdDev вычисляет среднюю и  ср.кв отклонение.ключи stat: min,max,mean,dev,deriv
func StdDev(x []float64) (mean, variance float64, stat map[string]float64) {
	var sm float64
	var min float64 = 99999999.0
	var max float64 = 0.0
	var deriv float64 = 0.0
	stat = make(map[string]float64)
	stat["min"] = 0.0
	stat["max"] = 0.0
	stat["mean"] = 0.0
	stat["dev"] = 0.0
	stat["deriv"] = 0.0
	if len(x) == 0 {
		return 0.0, 0.0, stat
	}
	stat["deriv"] = x[0]
	for k, v := range x {
		sm = sm + v
		if min > v {
			min = v
		}
		if max < v {
			max = v
		}
		if k > 1 {
			deriv = math.Max(deriv, math.Abs(v-x[k-1]))
		}
	}
	mean = sm / float64(len(x))
	stat["min"] = min
	stat["max"] = max
	stat["mean"] = mean
	stat["deriv"] = deriv
	if len(x) == 1 {
		return mean, 0.0, stat
	}
	var (
		ss           float64
		compensation float64
	)

	for _, v := range x {
		d := v - mean
		ss += d * d
		compensation += d
	}
	variance = (ss - compensation*compensation/float64(len(x))) / float64(len(x)-1)
	dv := math.Sqrt(variance)
	stat["dev"] = dv
	return mean, dv, stat

}

//TrainRBF производит один цикл обучения сети, вычисляет веса и возвращает ошибку сети
func (rbf *RBFNetwork) TrainRBF(input []float64, output []float64) float64 {
	rc := 0
	//mx := make([]float64, rbf.Inputs*rbf.Hiddens)
	for i := 0; i < len(input); i++ {
		for n := 0; n < rbf.Hiddens; n++ {
			mX[rc] = Gaussian(input[i], rbf.Centers[n], rbf.Spreads[n])
			rc++
		}
	}
	//вычисляем веса скрытого слоя W = (Ht*H)^-1*Ht*Y
	//H матрица входного слоя, где каждое значение вычиляется по распределению Гаусса
	//Ht транспорированная матрица H
	//(Ht*H)^-1 обратная матрица матрицы произведений Ht и H
	//Y матрица выхода нейронной сети = output
	//итак
	//входная матрица Для каждого входа заполняем матрицу распределением Гаусса
	h := mat.NewDense(rbf.Inputs, rbf.Hiddens, mX)
	//matPrint(m)
	//log.Println(mat.Formatted(m))
	//log.Printf("A :\n%v\n\n", mat.Formatted(m, mat.Prefix(""), mat.Excerpt(0)))
	//временная матрица
	m := mat.NewDense(rbf.Hiddens, rbf.Hiddens, nil)
	m.Mul(h.T(), h)
	//log.Println(mat.Formatted(m))
	ih := mat.NewDense(rbf.Hiddens, rbf.Hiddens, nil)
	ih.Inverse(m)
	//log.Println(mat.Formatted(ih))
	m = mat.NewDense(rbf.Hiddens, rbf.Inputs, nil)
	m.Mul(ih, h.T())

	y := mat.NewDense(rbf.Inputs, 1, output)
	//веса
	w := mat.NewDense(rbf.Hiddens, 1, nil)
	w.Mul(m, y)
	copy(rbf.WeightOutput, w.RawMatrix().Data)
	//log.Println(mat.Formatted(w))

	//ошибка сети
	//выход это сумма выходов каждого нейрона, т.е sum(W*I)
	m = mat.NewDense(rbf.Inputs, 1, nil)
	m.Mul(h, w)
	target := m.RawMatrix().Data
	errSum := 0.0
	for i := 0; i < len(output); i++ {
		err := output[i] - target[i]
		errSum += 0.5 * err * err
	}
	return errSum
}

//Predict прогнозирование на основе входных данных
func (rbf *RBFNetwork) Predict(input []float64) []float64 {
	rc := 0
	mx := make([]float64, len(input)*rbf.Hiddens)
	for i := 0; i < len(input); i++ {
		for n := 0; n < rbf.Hiddens; n++ {
			mx[rc] = Gaussian(input[i], rbf.Centers[n], rbf.Spreads[n])
			//log.Println(n*i+n)
			rc++
		}
	}
	h := mat.NewDense(len(input), rbf.Hiddens, mx)
	w := mat.NewDense(rbf.Hiddens, 1, rbf.WeightOutput)
	m := mat.NewDense(len(input), 1, nil)
	m.Mul(h, w)
	return m.RawMatrix().Data
}

//Train обучение сети
func (rbf *RBFNetwork) Train(input []float64, output []float64, epoches int) float64 {
	if len(input) != len(output) {
		return -1.0
	}
	sigma := rbf.Spread
	step := rbf.Spread * 0.9
	oprev := 99999999.99
	for ep := 0; ep < epoches && math.Abs(step) > 0.0005; ep++ {
		//изменяем сигма до минимализации ошибки
		rbf.SetSpread(sigma)
		o := rbf.TrainRBF(input, output)
		if o > oprev {
			step = -0.7 * step
			//log.Printf("o= %v prev= %v", o, oprev)
		}
		//log.Printf("sigma= %v step= %v o= %v prev= %v\n", sigma, step, o, oprev)
		sigma = math.Abs(sigma + step)
		oprev = o
		if oprev == 0 {
			break
		}
	}
	//теперь оптимизируем ширину Spreads функций, для последних должно быть шире ибо влияние на прогноз больше
	/*
		for n := rbf.Hiddens - 1; n > rbf.Hiddens-6 && n > 0; n-- {
			sigma = rbf.GetISpread(n)
			step = 1.0
			for ep := 0; ep < epoches && math.Abs(step) > 0.0005; ep++ {
				//изменяем сигма до минимализации ошибки
				sigma = math.Abs(sigma + step)
				rbf.SetISpread(n, sigma)
				o := rbf.TrainRBF(input, output)
				if o > oprev {
					step = -0.7 * step
					//log.Printf("o= %v prev= %v", o, oprev)
				}
				oprev = o
			}
		}
	*/
	return oprev
}

//TrainW обучение сети с корректировкой по известным значениям outputW для inputW
func (rbf *RBFNetwork) TrainW(inp []float64, out []float64, cntpred int, epoches int) float64 {
	cnt := len(inp)
	input := inp[0 : cnt-cntpred]
	output := out[0 : cnt-cntpred]
	inputW := inp[cnt-cntpred:]
	outputW := out[cnt-cntpred:]
	sigma := rbf.Spread
	step := rbf.Spread * 0.9
	errnet := 9999999999.99
	for ep := 0; ep < epoches && math.Abs(step) > 0.0005; ep++ {
		//изменяем сигма до минимализации ошибки
		rbf.SetSpread(sigma)
		o := rbf.TrainRBF(input, output)
		if o > errnet {
			step = -0.8 * step
		}
		sigma = math.Abs(sigma + step)
		errnet = o
		if errnet == 0 {
			break
		}
	}
	step = 1.0
	errnet = 9999999999.99
	reterr := 0.0
	for ep := 0; ep < epoches && math.Abs(step) > 0.0005; ep++ {
		//изменяем сигма до минимализации ошибки
		rbf.SetSpread(sigma)
		o := rbf.TrainRBF(input, output)
		pred := rbf.Predict(inputW)
		//errPr := 0.0
		errPr := o
		reterr = 0
		for i := 0; i < len(outputW) && i < len(inputW); i++ {
			err := outputW[i] - pred[i]
			reterr = (reterr + err/outputW[i]) / float64(i+1)
			errPr += 0.5 * err * err
		}
		if errPr > errnet {
			step = -0.8 * step
		}
		sigma = math.Abs(sigma + step)
		errnet = errPr
		if errnet < 0.0000001 && errnet > -0.0000001 {
			break
		}
		//изменяем сигма до минимализации ошибки
		//rbf.SetSpread(sigma)
		//o:=rbf.TrainRBF(input, output)
	}
	//итак мы нашли sigma, теперь не меняем ее
	//увеличим количество входов
	return reterr
}

//Round округляет число
func Round(val float64) int {
	if val < 0 {
		return int(val - 0.5)
	}
	return int(val + 0.5)
}

//MakeCenters возвращаем массив для центров нейронной сети. hidd это во сколько раз меньше скрытых нейронов
func MakeCenters(input []float64, hidd int) []float64 {
	//var num int
	//коэф разряжения, центры функций приходятся на один из inputs
	//но не менее 2 скрытых нейронов
	var numhiddens int
	numhiddens = Round(float64(len(input)) / float64(hidd))
	if numhiddens < 2 {
		numhiddens = 2
	}
	//log.Printf("hidd=%v\n", hidd)
	//num = round(hidd*float64(len(input))) + 1
	centers := make([]float64, numhiddens)
	centers[0] = input[0]
	if numhiddens > 1 {
		centers[numhiddens-1] = input[len(input)-1]
		for i, z := len(input), numhiddens; i > 0 && z > 0; i-- {
			if ((i - len(input)) % hidd) == 0 {
				centers[z-1] = input[i-1]
				//log.Printf("i=%v ", i)
				z--
			}
		}
	}
	return centers
}

//MakeCenters2 возвращаем массив для центров нейронной сети. lencenters это количество скрытых нейронов
func MakeCenters2(input []float64, lencenters int) []float64 {
	num := len(input)
	centers := make([]float64, lencenters)
	centers[0] = input[0]
	for i := 1; i < lencenters; i++ {
		centers[i] = float64(i * num / (lencenters - 1))
	}
	//centers[lencenters-1] = input[num-1]
	return centers
}

//GetSigma вычисляет среднюю mean и сигму из предположения, что input имеет Гауссово распределение
func GetSigma(x []float64) (mean float64, sigma float64, stat map[string]float64) {
	var sm float64
	mean, sigma, stat = StdDev(x)
	if len(x) < 6 {
		return mean, sigma, stat
	}
	frec := make(map[float64]float64)
	for _, v := range x {
		//max = math.Max(v, max)
		//min = math.Min(v, min)
		sm = sm + v
		frec[v]++
	}
	//mean = sm / float64(len(x))
	//не гаусс
	if stat["min"] == stat["max"] {
		//sigma=max
		return mean, sigma, stat
	}
	inputs := make([]float64, len(frec))
	output := make([]float64, len(frec))
	centers := []float64{0.0}
	var max float64 = 0.0
	i := 0
	frecmean := 0.0
	for k := range frec {
		inputs[i] = k
		//max = math.Max(frec[k], max) //максимальное количество
		if frec[k] > max {
			max = frec[k]
			frecmean = k
		}
		i++
	}
	//не гаусс
	if int(frec[frecmean]*4) < len(frec) {
		//sigma=max
		return mean, sigma, stat
	}
	max = frec[frecmean]
	sort.Float64s(inputs)
	for k, v := range inputs {
		output[k] = frec[v] / max //приводим к 1
		if math.Abs(v-frecmean) < 1 {
			centers[0] = v
		}
	}
	r := NewRBFNetwork(len(inputs), 1, 3.0, centers)
	r.Train(inputs, output, 1000)
	sigma = r.Spread
	return frecmean, sigma, stat
}

func main() {

}
