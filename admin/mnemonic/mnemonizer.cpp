/*
 * Mnemonizer.cpp
 * Object to convert random ipv4 and ipv6 to a mnemonic.
 * The gist of the algorithm works like this:
 *      Checks if the ip is valid ipv4 or ipv6
 *      Adds a static salt to the string
 *      Hashes the ip and the salt with SHA1
 *      Splits the hash into 4 parts
 *      Gets one of the mnemonicStart and one of the mneomicends for each part
 *
 * To use it:
 *  First require it:
 * 	require('./path/mnemonics.node');
 *  then create an object with a salt like this:
 *      var mnem = new mnemonics.mnemonizer("This is an example salt[0._\Acd2*+ç_SAs]");
 *  The salt is reccommended to be SALTLENGTH long
 *  Then you can call Apply_mnemonic with any ip
 *      mnem.Apply_mnemonic("192.168.1.1");
 *  This will return
 *      The mnemonic if the argument passed is a valid ip
 *      NULL if the ip was invalid or there was some incorrect parameter, and an error message will be printed
 */
#include "mnemonizer.h"
#include <sstream> //String stream
#include <cstdlib> //strtol
#include <cmath> //floor
#include <openssl/sha.h>

using namespace v8;
const std::string MNEMONICSTARTS[] = {"", "k", "s", "t", "d", "n", "h", "b",
                                "p", "m", "f", "r", "g", "z", "l", "ch"};
const std::string MNEMONICENDS[] = {"a", "i", "u", "e", "o", "a", "i", "u",
                                "e", "o", "ya", "yi", "yu", "ye", "yo", "'"};
const char* const HEXS = "0123456789ABCDEF";
const int SALTLENGTH = 40; //feel free to change this if you know a better length


void mnemonizer::Init(Handle<Object> exports) {
  NanScope();

  Local<FunctionTemplate> tpl = NanNew<FunctionTemplate>(New);
  tpl->SetClassName(NanNew("mnemonizer"));
  tpl->InstanceTemplate()->SetInternalFieldCount(1);

  NODE_SET_PROTOTYPE_METHOD(tpl,"Apply_mnemonic",Apply_mnemonic);

  NanAssignPersistent(constructor,tpl->GetFunction());
  exports->Set(NanNew("mnemonizer"),tpl->GetFunction());
}

NAN_METHOD(mnemonizer::New){
  NanScope();

  if(args.IsConstructCall()){
    mnemonizer* obj;
    if(!args[0]->IsString()){
        printf("mnemonizer: Warning, invalid Salt, no salt will we used (non secure)\n");
        obj= new mnemonizer(std::string(""));
    }else{
        String::Utf8Value saltUTF(args[0]);
        obj = new mnemonizer(std::string(*saltUTF));
    }

    obj->Wrap(args.This());
    NanReturnValue(args.This());
  }else {
    Local<Function> cons = NanNew<Function>(constructor);
    NanReturnValue(cons->NewInstance());
  }
}

Persistent<Function> mnemonizer::constructor;
mnemonizer::mnemonizer(std::string salt)
{

    if(salt.length()<SALTLENGTH)
        printf("mnemonizer: Warning, salt should be larger, at least %d characters\n",SALTLENGTH);
    this->salt=salt;
}

mnemonizer::~mnemonizer()
{
}


bool mnemonizer::isIpv4(std::string ip){
    if(*ip.rbegin()=='.')
        return false;
    std::stringstream ipStream(ip);
    std::string part;
    int count =0;
    char *end;
    while(std::getline(ipStream,part,'.')){
        if(count>3 || part.empty())
            return false;
        int val = strtol(part.c_str(),&end,10);
        if(*end!= '\0' ||val<0 || val>255)
            return false;
        count++;
    }
    return true;
}

bool mnemonizer::isIpv6(std::string ip){
    std::string::reverse_iterator lastC=ip.rbegin();
    if(ip.length()<2 ||(lastC[0]==':' && lastC[1]!=':')) //ends with ::
        return false;

    bool gap = false;
    std::string::iterator firstC = ip.begin();
    if(firstC[0]==':' && firstC[1]==':'){ //starts with ::
        gap=true;
        ip.erase(0,2);
    }
    std::stringstream ipStream(ip);
    std::string part;
    int count =0;

    char *end;
    while(std::getline(ipStream,part,':')){
        if(part.empty()){
            if(gap) //If we already have a gap don't accept this one
                return false;
            gap = true;
        }
        int val = strtol(part.c_str(),&end,16);
        if(*end!= '\0' || val>0xFFFF)
            return false;
        count++;
    }
    if(!gap && count!=8)
        return false;
    return true;
}

std::string mnemonizer::hashToMem(unsigned char* hash){
    std::string result;
    result.reserve(10);
    for(int i=0;i<4;i++){
        std::string part = ucharToHex(hash+(i*5),10);
        unsigned int val = strtoul(part.c_str(),NULL,16);
        result.append(
                    MNEMONICSTARTS[(int)floor((val%256)/16)]+
                    MNEMONICENDS[val%16]
                );
    }
    return result;
}

std::string mnemonizer::ucharToHex(unsigned char* hashpart,int length_hex){
    std::string out;
    out.reserve(length_hex);
    for(int i=0;i <length_hex/2;i++)
    {
        unsigned char c = hashpart[i];
        out.push_back(HEXS[c>>4]);
        out.push_back(HEXS[c&15]);
    }
    return out;
}

NAN_METHOD(mnemonizer::Apply_mnemonic){
    NanScope();
    mnemonizer* obj = ObjectWrap::Unwrap<mnemonizer>(args.Holder());

    if(!args[0]->IsString())
        printf("mnemonizer: Wrong argument passed to Apply_mnemonic(Should be an String)\n");

    String::Utf8Value ipUTF(args[0]);
    std::string ip= std::string(*ipUTF);
    if(obj->isIpv4(ip) || obj->isIpv6(ip)){
        unsigned char out[SHA_DIGEST_LENGTH];
        ip.append(obj->salt);
        SHA1((unsigned char*)ip.c_str(),ip.length(),out);
        NanReturnValue(NanNew<String>(obj->hashToMem(out)));
    }
    NanReturnValue(NanNull());
}