# Shared helpers for the five-organisation lab network. Source, do not run.
#
#   Org1 DMT (also the licensed assembler)   peer0.org1 :7051   CA :7054
#   Org2 Police                               peer0.org2 :9051   CA :8054
#   Org3 Customs                              peer0.org3 :11051  CA :11054
#   Org4 VET / emission centres               peer0.org4 :13051  CA :13054
#   Org5 Insurers                             peer0.org5 :15051  CA :15054
#   Orderers (Raft): orderer :7050, orderer2 :7052, orderer3 :7056; CA :9054
FABRIC_HOME="${FABRIC_HOME:-$HOME/fabric-registry/fabric-samples}"
TN="$FABRIC_HOME/test-network"
CHANNEL="${CHANNEL_NAME:-registry}"
CC="${CHAINCODE_NAME:-vehicle-registry}"
export PATH="$FABRIC_HOME/bin:$PATH" FABRIC_CFG_PATH="$FABRIC_HOME/config" CORE_PEER_TLS_ENABLED=true
ORDERER_CA="$TN/organizations/ordererOrganizations/example.com/tlsca/tlsca.example.com-cert.pem"
ORDERER_ARGS=(-o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --tls --cafile "$ORDERER_CA")
ALL_ORGS=(1 2 3 4 5)
# Majority of five. Writes go to the acting agency's peer plus enough others to make three.
ENDORSE_DEFAULT=(1 2 3)

peer_port() { echo $((7051 + 2000 * ($1 - 1))); }        # 7051 9051 11051 13051 15051
ca_port()   { case "$1" in 1) echo 7054;; 2) echo 8054;; *) echo $((11054 + 2000 * ($1 - 3)));; esac; }
org_dir()   { echo "$TN/organizations/peerOrganizations/org$1.example.com"; }
peer_tls()  { echo "$(org_dir "$1")/tlsca/tlsca.org$1.example.com-cert.pem"; }

# org_of <identity>: which organisation issues this lab identity.
org_of() {
  case "$1" in
    police*|org2) echo 2 ;;
    customs*) echo 3 ;;
    vet*) echo 4 ;;
    insurer*) echo 5 ;;
    *) echo 1 ;;                                          # dmt, User1, assembler1
  esac
}

# as <identity>: point the peer CLI at that identity and its own organisation's peer.
# dmt|User1 = User1@org1, police1|org2 = User1@org2, admin<N> = Admin@orgN.
as() {
  local n user="$1"
  case "$1" in
    admin[1-5]) n="${1#admin}"; user=Admin ;;
    dmt) n=1; user=User1 ;;
    police1|org2) n=2; user=User1 ;;
    *) n=$(org_of "$1") ;;
  esac
  export CORE_PEER_LOCALMSPID="Org${n}MSP" CORE_PEER_ADDRESS="localhost:$(peer_port "$n")" \
         CORE_PEER_TLS_ROOTCERT_FILE="$(peer_tls "$n")" \
         CORE_PEER_MSPCONFIGPATH="$(org_dir "$n")/users/$user@org$n.example.com/msp"
  WHO="$1"
}

# peer_args <org>...: --peerAddresses/--tlsRootCertFiles pairs in PEER_ARGS.
peer_args() {
  PEER_ARGS=()
  for n in "$@"; do PEER_ARGS+=(--peerAddresses "localhost:$(peer_port "$n")" --tlsRootCertFiles "$(peer_tls "$n")"); done
}

# endorsers_for: the acting identity's own peer first, then others up to three.
endorsers_for() {
  local own="${CORE_PEER_LOCALMSPID//[!0-9]/}"
  local picked=("$own")
  for n in "${ENDORSE_DEFAULT[@]}" "${ALL_ORGS[@]}"; do
    [[ ${#picked[@]} -ge 3 ]] && break
    [[ " ${picked[*]} " == *" $n "* ]] || picked+=("$n")
  done
  echo "${picked[@]}"
}
